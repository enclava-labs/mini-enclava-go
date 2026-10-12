package rag

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"sort"
	"strings"
	"testing"
)

func TestExtractTextSupportedFormats(t *testing.T) {
	docx := testDOCX(t, `<w:document><w:body><w:p><w:r><w:t>Hello &amp; DOCX</w:t></w:r></w:p></w:body></w:document>`)
	cases := []struct {
		name     string
		fileName string
		mimeType string
		content  []byte
		want     []string
	}{
		{name: "text", fileName: "note.txt", mimeType: "text/plain", content: []byte("plain text phrase"), want: []string{"plain text phrase"}},
		{name: "markdown", fileName: "note.md", mimeType: "text/markdown", content: []byte("# Title\nmarkdown phrase"), want: []string{"Title", "markdown phrase"}},
		{name: "csv", fileName: "data.csv", mimeType: "text/csv", content: []byte("name,value\nalpha,beta\n"), want: []string{"name value", "alpha beta"}},
		{name: "json", fileName: "data.json", mimeType: "application/json", content: []byte(`{"title":"One","nested":{"body":"Two"}}`), want: []string{"title", "One", "nested", "Two"}},
		{name: "jsonl", fileName: "data.jsonl", mimeType: "application/jsonl", content: []byte("{\"title\":\"One\"}\n{\"body\":\"Two\"}\n"), want: []string{"One", "Two"}},
		{name: "html", fileName: "page.html", mimeType: "text/html", content: []byte(`<html><script>bad()</script><body><h1>Hello &amp; HTML</h1><p>RAG world</p></body></html>`), want: []string{"Hello & HTML", "RAG world"}},
		{name: "docx", fileName: "doc.docx", mimeType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", content: docx, want: []string{"Hello & DOCX"}},
		{name: "pdf", fileName: "doc.pdf", mimeType: "application/pdf", content: []byte("%PDF-1.4\n(Hello PDF phrase) Tj\n%%EOF"), want: []string{"Hello PDF phrase"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, err := ExtractText(tc.fileName, tc.mimeType, tc.content)
			if err != nil {
				t.Fatalf("ExtractText returned error: %v", err)
			}
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Fatalf("expected %q in extracted text %q", want, text)
				}
			}
		})
	}
}

func testDOCX(t *testing.T, documentXML string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	file, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("create docx entry: %v", err)
	}
	if _, err := file.Write([]byte(documentXML)); err != nil {
		t.Fatalf("write docx entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close docx: %v", err)
	}
	return buf.Bytes()
}

func TestChunkTextUsesOverlap(t *testing.T) {
	chunks := ChunkText("abcdefghijklmnopqrstuvwxyz", 10, 2)
	if len(chunks) < 3 {
		t.Fatalf("expected multiple chunks, got %#v", chunks)
	}
	if !strings.HasPrefix(chunks[1], "ij") {
		t.Fatalf("expected second chunk to include overlap, got %#v", chunks)
	}
	if got := ChunkText("short", 100, 10); len(got) != 1 || got[0] != "short" {
		t.Fatalf("short string: %#v", got)
	}
}

func TestChunkTextKeepsRunesAfterBacktrack(t *testing.T) {
	token := "DISTINCTIVETOKEN"
	text := strings.Repeat("a", 12) + "." + token + strings.Repeat("b", 40)
	chunks := ChunkText(text, 20, 4)
	if !strings.Contains(strings.Join(chunks, ""), token) {
		t.Fatalf("dropped token after boundary: %#v", chunks)
	}
}

func TestChunkTextAdvancesWhenOverlapNearChunkSize(t *testing.T) {
	text := "abcdef.ghijklmnopqrstuvwxyz0123456789!!"
	chunks := ChunkText(text, 10, 9)
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
	joined := strings.Join(chunks, "")
	if !strings.Contains(joined, "abcdef") || !strings.Contains(joined, "0123456789") {
		t.Fatalf("incomplete chunks: %#v", chunks)
	}
}

func TestExtractDOCXIncludesHeaderAndFooter(t *testing.T) {
	docx := testDOCXParts(t, map[string]string{
		"word/document.xml": `<w:t>Body text</w:t>`,
		"word/header1.xml":  `<w:t>Header text</w:t>`,
		"word/footer1.xml":  `<w:t>Footer text</w:t>`,
		"word/media/skip":   `not-extracted`,
	})
	text, err := ExtractText("doc.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", docx)
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	for _, want := range []string{"Body text", "Header text", "Footer text"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in %q", want, text)
		}
	}
	if strings.Contains(text, "not-extracted") {
		t.Fatalf("extracted non-text part: %q", text)
	}
}

func TestExtractDOCXRejectsExpandedLimit(t *testing.T) {
	payload := `<w:t>SECRET_DECLARED</w:t>`
	docx := patchZipUncompressedSize(t, testDOCX(t, payload), "word/document.xml", 32*1024*1024+1)
	text, err := ExtractText("bomb.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", docx)
	if text != "" || !errors.Is(err, errDOCXExpandedLimit) || err.Error() != "docx expanded content exceeds limit" {
		t.Fatalf("text=%q err=%v", text, err)
	}
	if strings.Contains(err.Error(), "SECRET_DECLARED") {
		t.Fatalf("error leaked archive bytes: %v", err)
	}

	body := `<w:t>BodyVisible</w:t>`
	footer := `<w:t>FOOTER_SECRET</w:t>`
	parts := testDOCXParts(t, map[string]string{
		"word/document.xml": body,
		"word/header1.xml":  `<w:t>Header</w:t>`,
		"word/footer1.xml":  footer,
	})
	text, err = extractDOCXLimited(parts, int64(len(body)))
	if text != "" || !errors.Is(err, errDOCXExpandedLimit) {
		t.Fatalf("aggregate text=%q err=%v", text, err)
	}
	if strings.Contains(err.Error(), "FOOTER_SECRET") || strings.Contains(err.Error(), "BodyVisible") {
		t.Fatalf("error leaked archive bytes: %v", err)
	}

	text, err = extractDOCXLimited(parts, int64(len(body)+len(footer)+len(`<w:t>Header</w:t>`)))
	if err != nil {
		t.Fatalf("parts within limit: %v", err)
	}
	for _, want := range []string{"BodyVisible", "Header", "FOOTER_SECRET"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in %q", want, text)
		}
	}
}

func TestReadAndCloseRejectsExtraByteAndCloses(t *testing.T) {
	rc := &trackCloser{Reader: strings.NewReader("0123456789SECRET")}
	b, err := readAndClose(rc, 9)
	if b != nil || !errors.Is(err, errDOCXExpandedLimit) || !rc.closed {
		t.Fatalf("b=%q err=%v closed=%v", b, err, rc.closed)
	}
	if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "0123456789") {
		t.Fatalf("error leaked bytes: %v", err)
	}

	rc = &trackCloser{Reader: errReader{}}
	if _, err := readAndClose(rc, 32); err == nil || !rc.closed {
		t.Fatalf("err=%v closed=%v", err, rc.closed)
	}
}

type trackCloser struct {
	io.Reader
	closed bool
}

func (t *trackCloser) Close() error {
	t.closed = true
	return nil
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func testDOCXParts(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		file, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create docx entry: %v", err)
		}
		if _, err := file.Write([]byte(parts[name])); err != nil {
			t.Fatalf("write docx entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close docx: %v", err)
	}
	return buf.Bytes()
}

func patchZipUncompressedSize(t *testing.T, data []byte, name string, size uint32) []byte {
	t.Helper()
	out := append([]byte(nil), data...)
	sig := []byte{0x50, 0x4b, 0x01, 0x02}
	patched := false
	for i := 0; i+46 <= len(out); i++ {
		if !bytes.Equal(out[i:i+4], sig) {
			continue
		}
		nameLen := int(binary.LittleEndian.Uint16(out[i+28 : i+30]))
		extraLen := int(binary.LittleEndian.Uint16(out[i+30 : i+32]))
		commentLen := int(binary.LittleEndian.Uint16(out[i+32 : i+34]))
		nameStart := i + 46
		nameEnd := nameStart + nameLen
		if nameEnd+extraLen+commentLen > len(out) {
			continue
		}
		if string(out[nameStart:nameEnd]) != name {
			continue
		}
		binary.LittleEndian.PutUint32(out[i+24:i+28], size)
		patched = true
		break
	}
	if !patched {
		t.Fatalf("central header for %s not found", name)
	}
	reader, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("reopen patched zip: %v", err)
	}
	for _, file := range reader.File {
		if file.Name == name && file.UncompressedSize64 != uint64(size) {
			t.Fatalf("declared size %d, want %d", file.UncompressedSize64, size)
		}
	}
	return out
}
