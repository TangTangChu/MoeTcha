package core

import (
	"errors"
	"fmt"
	"image"
	"log/slog"
	"runtime"
	"sync"
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
	Difficulty   Difficulty
	IPPolicy     IPPolicy
	Secure       SecurePolicy

	GridConcurrency     int
	MaxSourcePixels     int
	gridSem             chan struct{}
	gridSemOnce         sync.Once
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

type ChallengeResponse struct {
	SessionID string                `json:"session_id"`
	Type      ChallengeType         `json:"type"`
	Question  string                `json:"question"`
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

	chal, err := s.Engine.GenerateChallenge(kind, s.Difficulty)
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

	plan, err := s.Engine.buildGridImagePlan(req, s.Difficulty)
	if err != nil {
		return nil, err
	}

	sem := s.gridSemaphore()
	sem <- struct{}{}
	defer func() { <-sem }()

	maxPixels := s.maxSourcePixels()
	images := make([]image.Image, 0, len(plan.tiles))
	for _, tile := range plan.tiles {
		img, err := render.LoadImage(tile.meta.Path)
		if err != nil {
			return nil, fmt.Errorf("加载 Grid 图片 %s 失败: %w", globalGridImageID(tile.meta), err)
		}
		if px := imagePixels(img); px > maxPixels {
			return nil, gridImageRequestError("源图 %s 像素数 %d 超过上限 %d", globalGridImageID(tile.meta), px, maxPixels)
		}
		if plan.applyRenderer && s.Renderer != nil && s.Renderer.Pipeline != nil {
			img, err = s.Renderer.Pipeline.Apply(img)
			if err != nil {
				return nil, fmt.Errorf("处理 Grid 图片 %s 失败: %w", globalGridImageID(tile.meta), err)
			}
		}
		images = append(images, img)
	}

	composed, placements, err := render.ComposeGrid(images, plan.compose)
	if err != nil {
		return nil, fmt.Errorf("合成 Grid 图片失败: %w", err)
	}
	bytes, err := render.EncodeWebPStrict(composed, plan.quality)
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

func (s *Service) Verify(sessionID string, grid *GridVerifyRequest, click *ClickVerifyRequest, ctx VerifyContext) (VerifyResult, error) {
	if sessionID == "" {
		return VerifyResult{OK: false, Reason: "session_id 为空"}, nil
	}
	ss, ok := s.SessionStore.IncrementAttempt(sessionID)
	if !ok {
		return VerifyResult{OK: false, Reason: "session 不存在或已过期"}, nil
	}

	if s.Secure.MinVerifyInterval > 0 {
		if ss.CreatedAt.Add(s.Secure.MinVerifyInterval).After(time.Now()) {
			return VerifyResult{OK: false, Reason: "验证过快"}, nil
		}
	}

	if s.IPPolicy.Enabled && s.IPPolicy.RequireMatch {
		if ctx.IP == "" || ss.IP == "" || ctx.IP != ss.IP {
			return VerifyResult{OK: false, Reason: "IP 不匹配"}, nil
		}
	}

	if s.Secure.RequireUserAgent || s.Secure.RequireSameUserAgent {
		if ctx.UserAgent == "" || ss.UserAgent == "" {
			return VerifyResult{OK: false, Reason: "缺少 User-Agent"}, nil
		}
		if s.Secure.RequireSameUserAgent && ctx.UserAgent != ss.UserAgent {
			return VerifyResult{OK: false, Reason: "User-Agent 不匹配"}, nil
		}
	}

	if s.Secure.MaxAttemptsPerIP > 0 && s.Secure.MaxAttemptsWindow > 0 {
		if tracker, ok := s.SessionStore.(IPAttemptTracker); ok {
			if !tracker.AllowAttempt(ctx.IP, s.Secure.MaxAttemptsPerIP, s.Secure.MaxAttemptsWindow) {
				return VerifyResult{OK: false, Reason: "IP 尝试次数过多"}, nil
			}
		}
	}
	if s.Secure.MaxFailRatio > 0 && s.Secure.FailRatioWindow > 0 {
		if tracker, ok := s.SessionStore.(IPAttemptTracker); ok {
			if !tracker.AllowFailRatio(ctx.IP, s.Secure.MaxFailRatio, s.Secure.FailRatioWindow) {
				return VerifyResult{OK: false, Reason: "失败率过高"}, nil
			}
		}
	}
	if s.Secure.RateLimit.Enabled {
		if limiter, ok := s.SessionStore.(RateLimiter); ok {
			if !limiter.Allow(ctx, s.Secure.RateLimit) {
				if !s.Secure.RateLimit.SoftReject {
					return VerifyResult{OK: false, Reason: "访问过于频繁"}, nil
				}
			}
		}
	}
	if s.Secure.Token.Enabled {
		if verifier, ok := s.SessionStore.(TokenVerifier); ok {
			if err := verifier.VerifyToken(sessionID, ctx, s.Secure.Token); err != nil {
				return VerifyResult{OK: false, Reason: err.Error()}, nil
			}
		}
	}

	chal := ss.Challenge
	if chal == nil {
		return VerifyResult{OK: false, Reason: "challenge 缺失"}, nil
	}

	var result VerifyResult
	switch chal.Type {
	case ChallengeGrid:
		if grid == nil {
			return VerifyResult{OK: false, Reason: "缺少 grid 请求"}, nil
		}
		result = VerifyGrid(chal, *grid)
	case ChallengeClick:
		if click == nil {
			return VerifyResult{OK: false, Reason: "缺少 click 请求"}, nil
		}
		result = VerifyClick(chal, *click)
	default:
		return VerifyResult{OK: false, Reason: "未知 challenge 类型"}, nil
	}

	if result.OK {
		_ = s.SessionStore.Delete(sessionID)
		if tracker, ok := s.SessionStore.(IPAttemptTracker); ok {
			tracker.RecordOutcome(ctx.IP, true)
		}
		MetricsInstance.VerificationsOK.Add(1)
		slog.Info("verify_ok", "session_id", sessionID, "correct", result.Correct, "total", result.Total)
	}
	if !result.OK {
		if tracker, ok := s.SessionStore.(IPAttemptTracker); ok {
			tracker.RecordOutcome(ctx.IP, false)
		}
		if s.Secure.DeleteOnFailed {
			_ = s.SessionStore.Delete(sessionID)
		}
		MetricsInstance.VerificationsFail.Add(1)
		slog.Info("verify_fail", "session_id", sessionID, "reason", result.Reason)
	}

	return result, nil
}

type VerifyContext struct {
	IP        string
	UserAgent string
	Token     string
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
	bytes, err := render.EncodeWebP(img, 80)
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

func imagePixels(img image.Image) int {
	b := img.Bounds()
	return b.Dx() * b.Dy()
}
