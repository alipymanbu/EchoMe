// Package tts contains provider-independent text-to-speech orchestration.
package tts

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/justin/echome-be/internal/domain/ai"
	"github.com/justin/echome-be/internal/domain/ws"
)

const (
	defaultMinSegmentRunes = 12
	defaultMaxSegmentRunes = 100
	defaultMaxSegmentWait  = 800 * time.Millisecond
)

// SegmenterConfig controls when buffered LLM text is handed to TTS.
type SegmenterConfig struct {
	MinRunes int
	MaxRunes int
	MaxWait  time.Duration
}

func DefaultSegmenterConfig() SegmenterConfig {
	return SegmenterConfig{
		MinRunes: defaultMinSegmentRunes,
		MaxRunes: defaultMaxSegmentRunes,
		MaxWait:  defaultMaxSegmentWait,
	}
}

// StreamingProvider adds provider-independent sentence segmentation around a
// provider. The provider still owns its native streaming API and audio format.
type StreamingProvider struct {
	provider ai.TTSProvider
	config   SegmenterConfig
}

var _ ai.TTSProvider = (*StreamingProvider)(nil)

func NewStreamingProvider(provider ai.TTSProvider, config SegmenterConfig) *StreamingProvider {
	config = normalizeConfig(config)
	return &StreamingProvider{provider: provider, config: config}
}

func (p *StreamingProvider) HandleTTS(ctx context.Context, clientWS ws.WebSocketConn, textStream <-chan string, config ai.TTSConfig) error {
	return p.provider.HandleTTS(ctx, clientWS, SegmentTextStream(ctx, textStream, p.config), config)
}

// SegmentTextStream converts arbitrary LLM chunks into complete, speakable
// text segments. It preserves all input text and flushes unfinished text on
// timeout or when the source stream closes.
func SegmentTextStream(ctx context.Context, input <-chan string, config SegmenterConfig) <-chan string {
	output := make(chan string, 8)
	config = normalizeConfig(config)
	go func() {
		defer close(output)
		segmenter := NewSegmenter(config)
		var timer *time.Timer
		var timerC <-chan time.Time
		stopTimer := func() {
			if timer == nil {
				return
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timerC = nil
		}
		resetTimer := func() {
			stopTimer()
			if segmenter.Len() == 0 {
				return
			}
			if timer == nil {
				timer = time.NewTimer(config.MaxWait)
			} else {
				timer.Reset(config.MaxWait)
			}
			timerC = timer.C
		}
		send := func(text string) bool {
			text = CleanTextForSpeech(text)
			if text == "" {
				return true
			}
			select {
			case output <- text:
				return true
			case <-ctx.Done():
				return false
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-timerC:
				if !send(segmenter.Flush()) {
					return
				}
				stopTimer()
			case chunk, ok := <-input:
				if !ok {
					stopTimer()
					send(segmenter.Flush())
					return
				}
				for _, segment := range segmenter.Push(chunk) {
					if !send(segment) {
						return
					}
				}
				resetTimer()
			}
		}
	}()
	return output
}

// CleanTextForSpeech removes presentation-only characters from the copy sent
// to TTS. The original LLM text is still sent to the UI unchanged.
func CleanTextForSpeech(text string) string {
	var builder strings.Builder
	runes := []rune(text)
	for index, r := range runes {
		if isSpeechSymbol(r) || isMarkdownChar(r, runes, index) {
			continue
		}
		builder.WriteRune(r)
	}

	// Markdown links should be spoken as their visible label, not as a URL.
	cleaned := builder.String()
	cleaned = stripMarkdownLinks(cleaned)
	cleaned = stripURLs(cleaned)
	return strings.Join(strings.Fields(cleaned), " ")
}

func isSpeechSymbol(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF: // emoji and pictographs
		return true
	case r >= 0x2600 && r <= 0x27FF: // dingbats, arrows and miscellaneous symbols
		return true
	case r == 0xFE0E || r == 0xFE0F: // variation selectors
		return true
	case r >= 0x1F3FB && r <= 0x1F3FF: // emoji skin-tone modifiers
		return true
	default:
		return false
	}
}

func isMarkdownChar(r rune, text []rune, index int) bool {
	switch r {
	case '`', '*', '_', '~', '|':
		return true
	case '#':
		return index == 0 || text[index-1] == '\n' || text[index-1] == '\r'
	case '>':
		return index == 0 || text[index-1] == '\n' || text[index-1] == '\r'
	case '-':
		return index == 0 || text[index-1] == '\n' || text[index-1] == '\r'
	default:
		return false
	}
}

func stripMarkdownLinks(text string) string {
	var builder strings.Builder
	runes := []rune(text)
	for index := 0; index < len(runes); index++ {
		if runes[index] != '[' {
			builder.WriteRune(runes[index])
			continue
		}
		endLabel := index + 1
		for endLabel < len(runes) && runes[endLabel] != ']' {
			endLabel++
		}
		if endLabel >= len(runes) || endLabel+1 >= len(runes) || runes[endLabel+1] != '(' {
			builder.WriteRune(runes[index])
			continue
		}
		endURL := endLabel + 2
		for endURL < len(runes) && runes[endURL] != ')' {
			endURL++
		}
		if endURL >= len(runes) {
			builder.WriteRune(runes[index])
			continue
		}
		builder.WriteString(string(runes[index+1 : endLabel]))
		index = endURL
	}
	return builder.String()
}

func stripURLs(text string) string {
	var builder strings.Builder
	for _, field := range strings.Fields(text) {
		lower := strings.ToLower(field)
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "www.") {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte(' ')
		}
		builder.WriteString(field)
	}
	return builder.String()
}

// Segmenter is a small streaming sentence segmenter. It is deliberately
// provider-neutral: punctuation and timing determine segment boundaries, not
// the selected TTS vendor.
type Segmenter struct {
	config SegmenterConfig
	buffer []rune
}

func NewSegmenter(config SegmenterConfig) *Segmenter {
	config = normalizeConfig(config)
	return &Segmenter{config: config}
}

func normalizeConfig(config SegmenterConfig) SegmenterConfig {
	defaults := DefaultSegmenterConfig()
	if config.MinRunes <= 0 {
		config.MinRunes = defaults.MinRunes
	}
	if config.MaxRunes <= 0 {
		config.MaxRunes = defaults.MaxRunes
	}
	if config.MaxRunes < config.MinRunes {
		config.MaxRunes = config.MinRunes
	}
	if config.MaxWait <= 0 {
		config.MaxWait = defaults.MaxWait
	}
	return config
}

func (s *Segmenter) Len() int { return len(s.buffer) }

func (s *Segmenter) Push(text string) []string {
	s.buffer = append(s.buffer, []rune(text)...)
	return s.split(false)
}

func (s *Segmenter) Flush() string {
	text := strings.TrimSpace(string(s.buffer))
	s.buffer = s.buffer[:0]
	return text
}

func (s *Segmenter) split(force bool) []string {
	var segments []string
	start := 0
	for start < len(s.buffer) {
		remaining := s.buffer[start:]
		hardBoundary := -1
		for index, r := range remaining {
			if isHardBoundary(r) {
				hardBoundary = index + 1
				break
			}
			if index+1 >= s.config.MaxRunes {
				break
			}
		}
		if hardBoundary > 0 {
			segments = append(segments, strings.TrimSpace(string(remaining[:hardBoundary])))
			start += hardBoundary
			continue
		}

		if len(remaining) < s.config.MaxRunes {
			break
		}

		// No hard boundary was found before the limit. Prefer the latest
		// soft boundary within the limit, otherwise cut exactly at MaxRunes.
		cut := -1
		for index, r := range remaining[:s.config.MaxRunes] {
			if index+1 >= s.config.MinRunes && isSoftBoundary(r) {
				cut = index + 1
			}
		}
		if cut <= 0 {
			cut = s.config.MaxRunes
		}
		segments = append(segments, strings.TrimSpace(string(remaining[:cut])))
		start += cut
	}
	if force && start < len(s.buffer) {
		segments = append(segments, strings.TrimSpace(string(s.buffer[start:])))
		start = len(s.buffer)
	}
	if start > 0 {
		s.buffer = append([]rune(nil), s.buffer[start:]...)
	}
	return nonEmpty(segments)
}

func isHardBoundary(r rune) bool {
	return strings.ContainsRune("。！？!?;；\n\r", r)
}

func isSoftBoundary(r rune) bool {
	return strings.ContainsRune("，,、：:", r) || unicode.IsSpace(r)
}

func nonEmpty(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	return result
}
