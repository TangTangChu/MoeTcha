package core

import "sync/atomic"

type Metrics struct {
	ChallengesGenerated atomic.Int64
	GridImagesGenerated atomic.Int64
	VerificationsOK     atomic.Int64
	VerificationsFail   atomic.Int64
	AssetsServed        atomic.Int64
}

var MetricsInstance = &Metrics{}

func (m *Metrics) Snapshot() map[string]int64 {
	return map[string]int64{
		"challenges_generated":  m.ChallengesGenerated.Load(),
		"grid_images_generated": m.GridImagesGenerated.Load(),
		"verifications_ok":      m.VerificationsOK.Load(),
		"verifications_fail":    m.VerificationsFail.Load(),
		"assets_served":         m.AssetsServed.Load(),
	}
}
