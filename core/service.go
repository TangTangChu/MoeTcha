package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"moetcha/core/render"
)

type Service struct {
	Engine       *Engine
	SessionStore SessionStore
	AssetStore   AssetStore
	Renderer     *Renderer
	TTL          time.Duration
	MaxAttempts  int
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
}

type GridChallengePublic struct {
	Images []GridItemPublic `json:"images"`
}

type GridItemPublic struct {
	ImageID  string `json:"image_id"`
	AssetKey string `json:"asset_key"`
}

type ClickChallengePublic struct {
	Image ClickItemPublic `json:"image"`
}

type ClickItemPublic struct {
	ImageID  string `json:"image_id"`
	AssetKey string `json:"asset_key"`
}

func (s *Service) NewChallenge(kind ChallengeType) (*ChallengeResponse, error) {
	if s.Engine == nil || s.SessionStore == nil || s.AssetStore == nil || s.Renderer == nil {
		return nil, fmt.Errorf("service 组件未初始化")
	}

	chal, err := s.Engine.GenerateChallenge(kind)
	if err != nil {
		return nil, err
	}

	sessionID := randomHex(16)
	if sessionID == "" {
		return nil, fmt.Errorf("生成 session ID 失败")
	}

	now := time.Now()
	exp := now.Add(s.ttl())

	session := ChallengeSession{
		ID:          sessionID,
		Challenge:   chal,
		CreatedAt:   now,
		ExpiresAt:   exp,
		Attempts:    0,
		MaxAttempts: s.maxAttempts(),
	}
	if err := s.SessionStore.Save(session); err != nil {
		return nil, err
	}

	resp, err := s.buildResponse(chal, exp)
	if err != nil {
		_ = s.SessionStore.Delete(sessionID)
		return nil, err
	}
	resp.SessionID = sessionID
	resp.Type = chal.Type
	resp.Question = chal.Question
	return resp, nil
}

func (s *Service) Verify(sessionID string, grid *GridVerifyRequest, click *ClickVerifyRequest) (VerifyResult, error) {
	if sessionID == "" {
		return VerifyResult{OK: false, Reason: "session_id 为空"}, nil
	}
	ss, ok := s.SessionStore.IncrementAttempt(sessionID)
	if !ok {
		return VerifyResult{OK: false, Reason: "session 不存在或已过期"}, nil
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
	}

	return result, nil
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
		resp.Click = &ClickChallengePublic{Image: ClickItemPublic{ImageID: chal.Click.Image.ImageID, AssetKey: assetKey}}
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

func randomHex(n int) string {
	if n <= 0 {
		return ""
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}
