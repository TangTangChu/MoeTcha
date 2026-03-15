package render

import "image"

type Obfuscator interface {
	Apply(img image.Image) (image.Image, error)
}

type Pipeline struct {
	steps []Obfuscator
}

func NewPipeline(steps ...Obfuscator) *Pipeline {
	return &Pipeline{steps: steps}
}

func (p *Pipeline) Apply(img image.Image) (image.Image, error) {
	out := img
	for _, s := range p.steps {
		var err error
		out, err = s.Apply(out)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
