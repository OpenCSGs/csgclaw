package execution

import (
	"strings"
	"time"

	"github.com/tiktoken-go/tokenizer"
)

const (
	outputRateInterval = 250 * time.Millisecond
	outputRateWindow   = 2 * time.Second
	maxRateTextBytes   = 64 * 1024
)

type tokenSample struct {
	at    time.Time
	count int
}

type outputRateMeter struct {
	model   string
	codec   tokenizer.Codec
	text    strings.Builder
	samples []tokenSample
	last    time.Time
	rate    float64
	at      int64
}

func (m *outputRateMeter) setModel(model string) {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == m.model {
		return
	}
	m.reset()
	m.model = model
	if strings.HasPrefix(model, "qwen") {
		// The Qwen provider sends settlement usage. Use a multilingual tokenizer
		// for the live estimate when the provider's vocabulary is unavailable.
		m.codec, _ = tokenizer.Get(tokenizer.O200kBase)
	} else if strings.HasPrefix(model, "gpt-5.") || strings.HasPrefix(model, "gpt-5-") {
		m.codec, _ = tokenizer.Get(tokenizer.O200kBase)
	} else {
		m.codec, _ = tokenizer.ForModel(tokenizer.Model(model))
	}
}

func (m *outputRateMeter) reset() {
	m.text.Reset()
	m.samples = m.samples[:0]
	m.last = time.Time{}
	m.rate, m.at = 0, 0
}

func (m *outputRateMeter) observe(now time.Time, text string) bool {
	if m.codec == nil {
		return false
	}
	if now.Sub(m.last) > outputRateWindow || m.text.Len()+len(text) > maxRateTextBytes {
		m.reset()
	}
	if len(text) > maxRateTextBytes {
		return false
	}
	m.text.WriteString(text)
	interval := outputRateInterval
	if len(m.samples) == 1 {
		interval = 50 * time.Millisecond
	}
	if now.Sub(m.last) < interval {
		return false
	}
	m.last = now
	count, err := m.codec.Count(m.text.String())
	if err != nil {
		return false
	}
	for len(m.samples) > 0 && now.Sub(m.samples[0].at) > outputRateWindow {
		m.samples = m.samples[1:]
	}
	m.rate, m.at = 0, 0
	if len(m.samples) > 0 {
		first := m.samples[0]
		rate := float64(count-first.count) / now.Sub(first.at).Seconds()
		if rate > 0 && rate <= 10000 {
			m.rate, m.at = rate, now.UnixMilli()
		}
	}
	m.samples = append(m.samples, tokenSample{at: now, count: count})
	return true
}
