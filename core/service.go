package core

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"moetcha/core/render"
)

var ErrRateLimited = errors.New("访问过于频繁")

const (
	defaultMaxSourcePixels    = 16_000_000
	defaultGridConcurrencyMin = 1
)

type Service struct {
	Engine       *Engine
	SessionStore SessionStore
	AssetStore   AssetStore
	Renderer     *Renderer
	TTL          time.Duration
	MaxAttempts  int
	// Difficulty 初始难度；运行期可用 SetDifficulty 调整（控制台命令）。
	Difficulty Difficulty
	IPPolicy   IPPolicy
	Secure     SecurePolicy

	GridConcurrency int
	MaxSourcePixels int
	RenderQuality   int
	GridWebPMethod  int
	gridSem         chan struct{}
	gridSemOnce     sync.Once

	// diffAtomic 是运行期难度的原子槽：SetDifficulty 写入，difficulty() 优先读它，
	// 未设置过（初始字段）时回落 Difficulty。
	diffAtomic atomic.Pointer[Difficulty]
}

// CurrentDifficulty 返回当前生效的难度（运行期调整过则为新值，否则为初始值）。
func (s *Service) CurrentDifficulty() Difficulty {
	if p := s.diffAtomic.Load(); p != nil {
		return *p
	}
	return s.Difficulty
}

// SetDifficulty 运行期调整验证码难度，立即对后续请求生效（serve 控制台命令）。
func (s *Service) SetDifficulty(d Difficulty) {
	s.diffAtomic.Store(&d)
}

type IPPolicy struct {
	Enabled      bool
	RequireMatch bool
	MaxActive    int
}

type SecurePolicy struct {
	RequireUserAgent     bool
	DeleteOnFailed       bool
	MaxAttemptsPerIP     int
	MaxAttemptsWindow    time.Duration
	MinVerifyInterval    time.Duration
	MaxFailRatio         float64
	FailRatioWindow      time.Duration
	RequireSameUserAgent bool
	Token                TokenPolicy
	RateLimit            RateLimitPolicy
}

type TokenPolicy struct {
	Enabled        bool
	TTL            time.Duration
	SingleUse      bool
	BindIP         bool
	BindUserAgent  bool
	BindSession    bool
	BindIPPrefix   int
	SigningKey     string
	SigningKeyNext string
	RotationGrace  time.Duration
}

type RateLimitPolicy struct {
	Enabled    bool
	PerIPQPS   int
	PerIPBurst int
	PerUAQPS   int
	PerUABurst int
	BlockTTL   time.Duration
	SoftReject bool
}

type Renderer struct {
	Pipeline *render.Pipeline
}

// NewRenderer 依据渲染配置构造 Renderer。
// NoiseEnabled=false 时返回空管线，Apply 为直通，产出干净高质量图；
// 开启噪声时才装配 NoiseObfuscator。
func NewRenderer(cfg RenderConfig) *Renderer {
	if !cfg.NoiseEnabled || cfg.NoiseDensity <= 0 {
		return &Renderer{Pipeline: render.NewPipeline()}
	}
	return &Renderer{
		Pipeline: render.NewPipeline(render.NoiseObfuscator{
			Density: cfg.NoiseDensity,
			Seed:    cfg.NoiseSeed,
		}),
	}
}

type ChallengeResponse struct {
	SessionID string                `json:"session_id"`
	Type      ChallengeType         `json:"type"`
	Question  string                `json:"question"`
	ExpiresAt string                `json:"expires_at"`
	Grid      *GridChallengePublic  `json:"grid,omitempty"`
	Click     *ClickChallengePublic `json:"click,omitempty"`
	Token     string                `json:"token,omitempty"`
}

type GridChallengePublic struct {
	Images []GridItemPublic `json:"images"`
}

type GridItemPublic struct {
	ImageID  string `json:"image_id"`
	AssetKey string `json:"asset_key"`
}

type ClickChallengePublic struct {
	Image    ClickItemPublic `json:"image"`
	Required int             `json:"required,omitempty"` // 需点击的目标数量；0 表示全部
}

type ClickItemPublic struct {
	ImageID  string `json:"image_id"`
	AssetKey string `json:"asset_key"`
}

func (s *Service) NewChallenge(kind ChallengeType, ctx VerifyContext) (*ChallengeResponse, error) {
	if s.Engine == nil || s.SessionStore == nil || s.AssetStore == nil || s.Renderer == nil {
		return nil, fmt.Errorf("service 组件未初始化")
	}
	if err := s.checkRateLimit(ctx); err != nil {
		return nil, err
	}

	chal, err := s.Engine.GenerateChallenge(kind, s.CurrentDifficulty())
	if err != nil {
		return nil, err
	}

	sessionID := RandomHex(16)
	if sessionID == "" {
		return nil, fmt.Errorf("生成 session ID 失败")
	}

	now := time.Now()
	exp := now.Add(s.ttl())

	if s.IPPolicy.Enabled {
		if ctx.IP == "" {
			return nil, fmt.Errorf("缺少 IP")
		}
		if s.IPPolicy.MaxActive > 0 {
			if st, ok := s.SessionStore.(IPSessionTracker); ok {
				if err := st.ValidateActiveCount(ctx.IP, s.IPPolicy.MaxActive); err != nil {
					return nil, err
				}
			}
		}
	}
	if s.Secure.RequireUserAgent && ctx.UserAgent == "" {
		return nil, fmt.Errorf("缺少 User-Agent")
	}

	session := ChallengeSession{
		ID:          sessionID,
		Challenge:   chal,
		CreatedAt:   now,
		ExpiresAt:   exp,
		Attempts:    0,
		MaxAttempts: s.maxAttempts(),
		IP:          ctx.IP,
		UserAgent:   ctx.UserAgent,
	}
	if err := s.SessionStore.Save(session); err != nil {
		return nil, err
	}

	resp, err := s.buildResponse(chal, exp)
	if err != nil {
		_ = s.SessionStore.Delete(sessionID)
		return nil, err
	}
	if s.Secure.Token.Enabled {
		if signer, ok := s.SessionStore.(TokenSigner); ok {
			token, err := signer.SignToken(sessionID, ctx, s.Secure.Token)
			if err != nil {
				_ = s.SessionStore.Delete(sessionID)
				return nil, err
			}
			resp.Token = token
		}
	}
	resp.SessionID = sessionID
	resp.Type = chal.Type
	resp.Question = chal.Question
	resp.ExpiresAt = exp.Format(time.RFC3339)
	MetricsInstance.ChallengesGenerated.Add(1)
	slog.Info("challenge_created", "session_id", sessionID, "type", chal.Type, "tag", chal.Tag)
	return resp, nil
}

// GenerateGridImage 生成带编号的单张 Grid WebP 图片，并将其作为临时 asset 保存。
func (s *Service) GenerateGridImage(req GridImageGenerateRequest, ctx VerifyContext) (*GridImageGenerateResult, error) {
	if s == nil || s.Engine == nil || s.AssetStore == nil {
		return nil, fmt.Errorf("grid image service 组件未初始化")
	}
	if err := s.checkRateLimit(ctx); err != nil {
		return nil, err
	}

	plan, err := s.Engine.buildGridImagePlan(req, s.CurrentDifficulty())
	if err != nil {
		return nil, err
	}

	sem := s.gridSemaphore()
	sem <- struct{}{}
	defer func() { <-sem }()

	maxPixels := s.maxSourcePixels()
	images := make([]image.Image, 0, len(plan.tiles))
	// 源图只加载与大小校验；渲染/干扰管线放到合成后一次性应用，
	// 避免对每张全分辨率源图跑一次噪声（合成后画布比所有源图总像素小得多）。
	for _, tile := range plan.tiles {
		img, err := render.LoadImage(tile.meta.Path)
		if err != nil {
			return nil, fmt.Errorf("加载 Grid 图片 %s 失败: %w", globalGridImageID(tile.meta), err)
		}
		if px := imagePixels(img); px > maxPixels {
			return nil, gridImageRequestError("源图 %s 像素数 %d 超过上限 %d", globalGridImageID(tile.meta), px, maxPixels)
		}
		images = append(images, img)
	}

	composed, placements, err := render.ComposeGrid(images, plan.compose)
	if err != nil {
		return nil, fmt.Errorf("合成 Grid 图片失败: %w", err)
	}

	if plan.applyRenderer && s.Renderer != nil && s.Renderer.Pipeline != nil {
		if processed, perr := s.Renderer.Pipeline.Apply(composed); perr == nil && processed != nil {
			composed = toRGBA(processed)
		}
	}

	method := s.gridWebPMethod(composed.Bounds().Dx() * composed.Bounds().Dy())
	bytes, err := render.EncodeWebPStrict(composed, plan.quality, method)
	if err != nil {
		return nil, fmt.Errorf("编码 Grid WebP 失败: %w", err)
	}

	now := time.Now()
	ttl := s.ttl()
	if plan.temporaryTTLSeconds > 0 {
		ttl = time.Duration(plan.temporaryTTLSeconds) * time.Second
	}
	expiresAt := now.Add(ttl)
	assetKey, err := s.AssetStore.Save(Asset{
		Bytes:     bytes,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("保存 Grid 临时图片失败: %w", err)
	}
	MetricsInstance.GridImagesGenerated.Add(1)

	result := &GridImageGenerateResult{
		ID:              RandomHex(16),
		AssetKey:        assetKey,
		ContentType:     "image/webp",
		CreatedAt:       now,
		ExpiresAt:       expiresAt,
		Tag:             plan.tag,
		TagDisplay:      plan.tagDisplay,
		Question:        plan.question,
		Difficulty:      plan.difficulty,
		Seed:            plan.seed,
		ImageCount:      len(plan.tiles),
		CorrectCount:    len(plan.correctNumbers),
		CorrectNumbers:  append([]int(nil), plan.correctNumbers...),
		CorrectImageIDs: append([]string(nil), plan.correctImageIDs...),
		Width:           composed.Bounds().Dx(),
		Height:          composed.Bounds().Dy(),
		Rows:            plan.compose.Rows,
		Columns:         plan.compose.Columns,
		TileWidth:       plan.compose.TileWidth,
		TileHeight:      plan.compose.TileHeight,
		Gap:             plan.compose.Gap,
		Padding:         plan.compose.Padding,
		Fit:             plan.compose.Fit,
		Background:      formatGridColor(plan.compose.Background),
		Quality:         plan.quality,
		Shuffle:         plan.shuffle,
		ShowLabels:      plan.compose.ShowLabels,
		LabelScale:      plan.compose.LabelScale,
		LabelPosition:   plan.compose.LabelPosition,
		LabelColor:      formatGridColor(plan.compose.LabelForeground),
		LabelBackground: formatGridColor(plan.compose.LabelBackground),
		ApplyRenderer:   plan.applyRenderer,
		Tiles:           make([]GridImageGeneratedTile, 0, len(plan.tiles)),
	}
	if result.ID == "" {
		result.ID = assetKey
	}

	for i, tile := range plan.tiles {
		placement := placements[i]
		result.Tiles = append(result.Tiles, GridImageGeneratedTile{
			Number:  placement.Number,
			ImageID: globalGridImageID(tile.meta),
			File:    tile.meta.File,
			Tags:    append([]string(nil), tile.meta.Tags...),
			Correct: tile.correct,
			X:       placement.X,
			Y:       placement.Y,
			Width:   placement.Width,
			Height:  placement.Height,
		})
	}
	return result, nil
}

func (s *Service) checkRateLimit(ctx VerifyContext) error {
	if s == nil || !s.Secure.RateLimit.Enabled || s.SessionStore == nil {
		return nil
	}
	if limiter, ok := s.SessionStore.(RateLimiter); ok {
		if !limiter.Allow(ctx, s.Secure.RateLimit) && !s.Secure.RateLimit.SoftReject {
			return ErrRateLimited
		}
	}
	return nil
}

// Verify 处理一次校验请求。
// 返回 (VerifyResult, nil) 表示请求已被正常判定，result.Solved 表示通过与否。
// 返回 (_, *VerifyError) 表示请求级失败（会话过期、限流、绑定校验不过等），
// 此时根本没进入判定，handler 据错误码映射 HTTP 状态码。
func (s *Service) Verify(sessionID string, grid *GridVerifyRequest, click *ClickVerifyRequest, ctx VerifyContext) (VerifyResult, error) {
	if sessionID == "" {
		return VerifyResult{}, NewVerifyError(CodeEmptySession, "session_id 为空")
	}
	ss, ok := s.SessionStore.IncrementAttempt(sessionID)
	if !ok {
		return VerifyResult{}, NewVerifyError(CodeSessionExpired, "会话不存在或已过期")
	}

	if s.Secure.MinVerifyInterval > 0 {
		if ss.CreatedAt.Add(s.Secure.MinVerifyInterval).After(time.Now()) {
			return VerifyResult{}, NewVerifyError(CodeTooFast, "验证过快，请稍后再试")
		}
	}

	if s.IPPolicy.Enabled && s.IPPolicy.RequireMatch {
		if ctx.IP == "" || ss.IP == "" || ctx.IP != ss.IP {
			return VerifyResult{}, NewVerifyError(CodeIPMismatch, "IP 与签发时不一致")
		}
	}

	if s.Secure.RequireUserAgent || s.Secure.RequireSameUserAgent {
		if ctx.UserAgent == "" || ss.UserAgent == "" {
			return VerifyResult{}, NewVerifyError(CodeMissingUA, "缺少 User-Agent")
		}
		if s.Secure.RequireSameUserAgent && ctx.UserAgent != ss.UserAgent {
			return VerifyResult{}, NewVerifyError(CodeUAMismatch, "User-Agent 与签发时不一致")
		}
	}

	if s.Secure.MaxAttemptsPerIP > 0 && s.Secure.MaxAttemptsWindow > 0 {
		if tracker, ok := s.SessionStore.(IPAttemptTracker); ok {
			if !tracker.AllowAttempt(ctx.IP, s.Secure.MaxAttemptsPerIP, s.Secure.MaxAttemptsWindow) {
				return VerifyResult{}, NewVerifyError(CodeTooManyAttempts, "IP 尝试次数过多")
			}
		}
	}
	if s.Secure.MaxFailRatio > 0 && s.Secure.FailRatioWindow > 0 {
		if tracker, ok := s.SessionStore.(IPAttemptTracker); ok {
			if !tracker.AllowFailRatio(ctx.IP, s.Secure.MaxFailRatio, s.Secure.FailRatioWindow) {
				return VerifyResult{}, NewVerifyError(CodeHighFailRatio, "失败率过高")
			}
		}
	}
	if s.Secure.RateLimit.Enabled {
		if limiter, ok := s.SessionStore.(RateLimiter); ok {
			if !limiter.Allow(ctx, s.Secure.RateLimit) {
				if !s.Secure.RateLimit.SoftReject {
					return VerifyResult{}, NewVerifyError(CodeRateLimited, "访问过于频繁")
				}
			}
		}
	}
	if s.Secure.Token.Enabled {
		if verifier, ok := s.SessionStore.(TokenVerifier); ok {
			if err := verifier.VerifyToken(sessionID, ctx, s.Secure.Token); err != nil {
				rid := ctx.RequestID
				slog.Warn("token_verify_failed", "request_id", rid, "session_id", sessionID, "error", err.Error())
				return VerifyResult{}, NewVerifyError(CodeTokenInvalid, "Token 无效或已过期")
			}
		}
	}

	chal := ss.Challenge
	if chal == nil {
		return VerifyResult{}, NewVerifyError(CodeChallengeMissing, "challenge 缺失")
	}

	var result VerifyResult
	switch chal.Type {
	case ChallengeGrid:
		if grid == nil {
			return VerifyResult{}, NewVerifyError(CodeMissingGrid, "缺少 grid 请求体")
		}
		result = VerifyGrid(chal, *grid)
	case ChallengeClick:
		if click == nil {
			return VerifyResult{}, NewVerifyError(CodeMissingClick, "缺少 click 请求体")
		}
		result = VerifyClick(chal, *click)
	default:
		return VerifyResult{}, NewVerifyError(CodeUnknownType, "未知 challenge 类型")
	}

	if result.Solved {
		_ = s.SessionStore.Delete(sessionID)
		if tracker, ok := s.SessionStore.(IPAttemptTracker); ok {
			tracker.RecordOutcome(ctx.IP, true)
		}
		MetricsInstance.VerificationsOK.Add(1)
		slog.Info("verify_ok", "session_id", sessionID, "correct", result.Correct, "total", result.Total)
	} else {
		if tracker, ok := s.SessionStore.(IPAttemptTracker); ok {
			tracker.RecordOutcome(ctx.IP, false)
		}
		if s.Secure.DeleteOnFailed {
			_ = s.SessionStore.Delete(sessionID)
		}
		MetricsInstance.VerificationsFail.Add(1)
		slog.Info("verify_fail", "session_id", sessionID, "code", result.Code, "reason", result.Reason)
	}

	return result, nil
}

type VerifyContext struct {
	IP        string
	UserAgent string
	Token     string
	RequestID string
}

type IPSessionTracker interface {
	ValidateActiveCount(ip string, max int) error
}

type IPAttemptTracker interface {
	AllowAttempt(ip string, max int, window time.Duration) bool
	RecordOutcome(ip string, ok bool)
	AllowFailRatio(ip string, ratio float64, window time.Duration) bool
}

type TokenSigner interface {
	SignToken(sessionID string, ctx VerifyContext, policy TokenPolicy) (string, error)
}

type TokenVerifier interface {
	VerifyToken(sessionID string, ctx VerifyContext, policy TokenPolicy) error
}

type RateLimiter interface {
	Allow(ctx VerifyContext, policy RateLimitPolicy) bool
}

func (s *Service) buildResponse(chal *ChallengeInternal, exp time.Time) (*ChallengeResponse, error) {
	if chal == nil {
		return nil, fmt.Errorf("challenge 为空")
	}

	resp := &ChallengeResponse{}
	if chal.Type == ChallengeGrid && chal.Grid != nil {
		items := make([]GridItemPublic, 0, len(chal.Grid.Images))
		for _, it := range chal.Grid.Images {
			assetKey, err := s.renderAndStore(it.Path, exp)
			if err != nil {
				return nil, err
			}
			items = append(items, GridItemPublic{ImageID: it.ImageID, AssetKey: assetKey})
		}
		resp.Grid = &GridChallengePublic{Images: items}
		return resp, nil
	}

	if chal.Type == ChallengeClick && chal.Click != nil {
		assetKey, err := s.renderAndStore(chal.Click.Image.Path, exp)
		if err != nil {
			return nil, err
		}
		required := chal.Click.Required
		if required <= 0 {
			required = len(chal.Click.Regions)
		}
		resp.Click = &ClickChallengePublic{Image: ClickItemPublic{ImageID: chal.Click.Image.ImageID, AssetKey: assetKey}, Required: required}
		return resp, nil
	}

	return nil, fmt.Errorf("challenge 数据不完整")
}

func (s *Service) renderAndStore(path string, exp time.Time) (string, error) {
	img, err := render.LoadImage(path)
	if err != nil {
		return "", err
	}
	if s.Renderer != nil && s.Renderer.Pipeline != nil {
		img, err = s.Renderer.Pipeline.Apply(img)
		if err != nil {
			return "", err
		}
	}
	bytes, err := render.EncodeWebP(img, s.renderQuality())
	if err != nil {
		return "", err
	}
	asset := Asset{Bytes: bytes, CreatedAt: time.Now(), ExpiresAt: exp}
	return s.AssetStore.Save(asset)
}

func (s *Service) ttl() time.Duration {
	if s.TTL <= 0 {
		return 2 * time.Minute
	}
	return s.TTL
}

func (s *Service) maxAttempts() int {
	if s.MaxAttempts <= 0 {
		return 3
	}
	return s.MaxAttempts
}

func (s *Service) gridSemaphore() chan struct{} {
	s.gridSemOnce.Do(func() {
		n := s.GridConcurrency
		if n <= 0 {
			n = runtime.NumCPU()
		}
		if n < defaultGridConcurrencyMin {
			n = defaultGridConcurrencyMin
		}
		s.gridSem = make(chan struct{}, n)
	})
	return s.gridSem
}

func (s *Service) maxSourcePixels() int {
	if s.MaxSourcePixels > 0 {
		return s.MaxSourcePixels
	}
	return defaultMaxSourcePixels
}

// renderQuality 返回 /challenge 响应图的 WebP 编码质量。0 表示默认 80。
func (s *Service) renderQuality() float32 {
	q := s.RenderQuality
	if q <= 0 {
		q = 80
	}
	if q > 100 {
		q = 100
	}
	return float32(q)
}

// gridWebPMethod 返回 /grid/generate 的 libwebp effort 方法。
// s.GridWebPMethod>0 时直接使用（钳制到 1~6）；0 表示按合成图像素数自动选档：
// 小图用高质量档（4，反正快），中等图用平衡档（2），超大图用快速档（1），
// 避免大网格在 method=4 下编码耗时几秒。
func (s *Service) gridWebPMethod(pixels int) int {
	m := s.GridWebPMethod
	if m > 0 {
		if m > 6 {
			m = 6
		}
		return m
	}
	switch {
	case pixels <= 250_000:
		return 4
	case pixels <= 4_000_000:
		return 2
	default:
		return 1
	}
}

// toRGBA 将任意 image.Image 转为 *image.RGBA（合成后管线输出可能是非 RGBA）。
func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok {
		return r
	}
	b := img.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, img, b.Min, draw.Src)
	return out
}

func imagePixels(img image.Image) int {
	b := img.Bounds()
	return b.Dx() * b.Dy()
}
