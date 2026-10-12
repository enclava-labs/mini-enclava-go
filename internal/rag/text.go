package rag

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

const docxExpandedByteLimit = 32 * 1024 * 1024

var errDOCXExpandedLimit = errors.New("docx expanded content exceeds limit")

var (
	scriptStyleRE = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	tagRE         = regexp.MustCompile(`(?s)<[^>]+>`)
	xmlTagRE      = regexp.MustCompile(`(?s)<[^>]+>`)
	spaceRE       = regexp.MustCompile(`[ \t\r\n]+`)
)

func ExtractText(fileName, mimeType string, content []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	mime := strings.ToLower(strings.TrimSpace(mimeType))

	switch {
	case strings.HasPrefix(mime, "text/plain") || ext == ".txt":
		return normalizeText(string(content)), nil
	case strings.Contains(mime, "markdown") || ext == ".md" || ext == ".markdown":
		return normalizeText(string(content)), nil
	case strings.Contains(mime, "csv") || ext == ".csv":
		return extractCSV(content)
	case strings.Contains(mime, "json") || ext == ".json" || ext == ".jsonl":
		return extractJSON(content)
	case strings.Contains(mime, "html") || ext == ".html" || ext == ".htm":
		return extractHTML(content), nil
	case strings.Contains(mime, "wordprocessingml") || ext == ".docx":
		return extractDOCX(content)
	case strings.Contains(mime, "pdf") || ext == ".pdf":
		return extractPDFBestEffort(content), nil
	default:
		if looksText(content) {
			return normalizeText(string(content)), nil
		}
		return "", fmt.Errorf("unsupported document type: %s", ext)
	}
}

func extractCSV(content []byte) (string, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	reader.FieldsPerRecord = -1
	var out strings.Builder
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parse csv: %w", err)
		}
		out.WriteString(strings.Join(record, " "))
		out.WriteByte('\n')
	}
	return normalizeText(out.String()), nil
}

func extractJSON(content []byte) (string, error) {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return "", nil
	}
	var v any
	if json.Unmarshal(trimmed, &v) == nil {
		return normalizeText(flattenJSON(v)), nil
	}
	lines := strings.Split(string(content), "\n")
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var item any
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return "", fmt.Errorf("parse json/jsonl: %w", err)
		}
		values = append(values, flattenJSON(item))
	}
	return normalizeText(strings.Join(values, "\n")), nil
}

func flattenJSON(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	case float64, bool:
		return fmt.Sprint(value)
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			parts = append(parts, flattenJSON(item))
		}
		return strings.Join(parts, " ")
	case map[string]any:
		parts := make([]string, 0, len(value)*2)
		for k, item := range value {
			parts = append(parts, k, flattenJSON(item))
		}
		return strings.Join(parts, " ")
	default:
		return fmt.Sprint(value)
	}
}

func extractHTML(content []byte) string {
	text := scriptStyleRE.ReplaceAllString(string(content), " ")
	text = tagRE.ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	return normalizeText(text)
}

func extractDOCX(content []byte) (string, error) {
	return extractDOCXLimited(content, docxExpandedByteLimit)
}

func extractDOCXLimited(content []byte, limit int64) (string, error) {
	if limit < 0 {
		limit = 0
	}
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", fmt.Errorf("open docx: %w", err)
	}
	var out strings.Builder
	remaining := limit
	for _, file := range reader.File {
		if !isDOCXTextPart(file.Name) {
			continue
		}
		if file.UncompressedSize64 > uint64(remaining) {
			return "", errDOCXExpandedLimit
		}
		b, err := readZipEntry(file, remaining)
		if err != nil {
			return "", err
		}
		remaining -= int64(len(b))
		out.WriteString(html.UnescapeString(xmlTagRE.ReplaceAllString(string(b), " ")))
		out.WriteByte('\n')
	}
	text := normalizeText(out.String())
	if text == "" {
		return "", fmt.Errorf("docx has no extractable text")
	}
	return text, nil
}

func isDOCXTextPart(name string) bool {
	return name == "word/document.xml" || strings.HasPrefix(name, "word/header") || strings.HasPrefix(name, "word/footer")
}

func readZipEntry(file *zip.File, remaining int64) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("read docx entry: %w", err)
	}
	return readAndClose(rc, remaining)
}

func readAndClose(rc io.ReadCloser, remaining int64) ([]byte, error) {
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(io.LimitReader(rc, remaining+1))
	if err != nil {
		return nil, fmt.Errorf("read docx xml: %w", err)
	}
	if int64(len(b)) > remaining {
		return nil, errDOCXExpandedLimit
	}
	return b, nil
}

func extractPDFBestEffort(content []byte) string {
	raw := string(content)
	matches := regexp.MustCompile(`\(([^)]{2,})\)`).FindAllStringSubmatch(raw, -1)
	parts := make([]string, 0, len(matches))
	for _, match := range matches {
		parts = append(parts, strings.ReplaceAll(match[1], `\)`, ")"))
	}
	if len(parts) > 0 {
		return normalizeText(strings.Join(parts, " "))
	}
	var out strings.Builder
	for _, r := range string(content) {
		if r == '\n' || r == '\r' || r == '\t' || unicode.IsPrint(r) {
			out.WriteRune(r)
		} else {
			out.WriteByte(' ')
		}
	}
	return normalizeText(out.String())
}

func looksText(content []byte) bool {
	if len(content) == 0 {
		return false
	}
	limit := len(content)
	if limit > 1024 {
		limit = 1024
	}
	bad := 0
	for _, b := range content[:limit] {
		if b == 0 {
			return false
		}
		if b < 8 || (b > 13 && b < 32) {
			bad++
		}
	}
	return bad < limit/20
}

func normalizeText(value string) string {
	value = strings.TrimSpace(value)
	value = spaceRE.ReplaceAllString(value, " ")
	return value
}
