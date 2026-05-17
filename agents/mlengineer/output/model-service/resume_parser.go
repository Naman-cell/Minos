package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

func ExtractResumeText(filename string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".docx":
		return extractDOCXText(data)
	case ".pdf":
		return extractPDFText(data)
	default:
		return "", fmt.Errorf("unsupported resume file type %q; expected .pdf or .docx", ext)
	}
}

func extractDOCXText(data []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		return readWordDocumentXML(rc)
	}
	return "", fmt.Errorf("docx missing word/document.xml")
}

func readWordDocumentXML(r io.Reader) (string, error) {
	decoder := xml.NewDecoder(r)
	var parts []string
	inText := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == "t" {
				inText = true
			}
			if t.Name.Local == "tab" || t.Name.Local == "br" || t.Name.Local == "p" {
				parts = append(parts, " ")
			}
		case xml.EndElement:
			if t.Name.Local == "t" {
				inText = false
				parts = append(parts, " ")
			}
		case xml.CharData:
			if inText {
				parts = append(parts, string(t))
			}
		}
	}
	return normalizeWhitespace(strings.Join(parts, "")), nil
}

func extractPDFText(data []byte) (string, error) {
	raw := string(data)
	var parts []string
	literalRE := regexp.MustCompile(`\((?:\\.|[^\\)])*\)`)
	for _, match := range literalRE.FindAllString(raw, -1) {
		unescaped := strings.TrimSuffix(strings.TrimPrefix(match, "("), ")")
		unescaped = strings.ReplaceAll(unescaped, `\(`, "(")
		unescaped = strings.ReplaceAll(unescaped, `\)`, ")")
		unescaped = strings.ReplaceAll(unescaped, `\\`, `\`)
		if looksLikeText(unescaped) {
			parts = append(parts, unescaped)
		}
	}
	text := normalizeWhitespace(strings.Join(parts, " "))
	if len(text) >= 20 {
		return text, nil
	}

	printable := printableRuns(data)
	if len(printable) >= 20 {
		return printable, nil
	}
	return "", fmt.Errorf("could not extract text from pdf; scanned or compressed PDFs need OCR or pdftotext")
}

func printableRuns(data []byte) string {
	var parts []string
	var current strings.Builder
	for _, b := range data {
		r := rune(b)
		if r == '\n' || r == '\r' || r == '\t' || (r >= 32 && r <= 126) {
			current.WriteRune(r)
			continue
		}
		if current.Len() >= 8 {
			parts = append(parts, current.String())
		}
		current.Reset()
	}
	if current.Len() >= 8 {
		parts = append(parts, current.String())
	}
	return normalizeWhitespace(strings.Join(parts, " "))
}

func looksLikeText(text string) bool {
	letters := 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	return letters >= 3
}

func normalizeWhitespace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
