package rag

import (
	"strings"
	"unicode/utf8"
)

func ChunkText(text string, chunkSize, overlap int) []string {
	text = normalizeText(text)
	if text == "" {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 5
	}

	runes := []rune(text)
	if len(runes) <= chunkSize {
		return []string{text}
	}
	chunks := []string{}
	for start := 0; start < len(runes); {
		end := start + chunkSize
		if end >= len(runes) {
			end = len(runes)
		} else {
			end = backtrackToBoundary(runes, start, end)
		}
		chunk := strings.TrimSpace(string(runes[start:end]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end >= len(runes) {
			break
		}
		next := end - overlap
		if next <= start {
			next = end
		}
		start = next
	}
	return chunks
}

func backtrackToBoundary(runes []rune, start, end int) int {
	min := start + (end-start)/2
	for i := end; i > min; i-- {
		switch runes[i-1] {
		case '.', '!', '?', '\n', ' ':
			return i
		}
	}
	return end
}

func tokenEstimate(text string) int {
	if text == "" {
		return 0
	}
	// Rough operational estimate, used only for local chunk metadata.
	return (utf8.RuneCountInString(text) + 3) / 4
}
