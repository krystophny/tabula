package web

import (
	"bytes"
	stdhtml "html"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	rendererhtml "github.com/yuin/goldmark/renderer/html"
)

var assistantMarkdownRenderer = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		extension.DefinitionList,
		extension.Footnote,
		highlighting.NewHighlighting(
			highlighting.WithStyle("github"),
			highlighting.WithFormatOptions(
				chromahtml.WithClasses(false),
			),
		),
	),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
	goldmark.WithRendererOptions(
		rendererhtml.WithHardWraps(),
	),
)

func renderMarkdownHTML(markdown string) string {
	source := strings.TrimSpace(markdown)
	if source == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := assistantMarkdownRenderer.Convert([]byte(source), &buf); err != nil {
		return "<p>" + stdhtml.EscapeString(source) + "</p>"
	}
	return strings.TrimSpace(buf.String())
}

func assistantRenderHTML(markdown string) string {
	cleaned := strings.TrimSpace(stripLangTags(markdown))
	if cleaned == "" {
		return ""
	}
	return renderMarkdownHTML(cleaned)
}

func assistantRenderChatCommand(turnID, outputMode, markdown string) map[string]interface{} {
	renderedHTML := assistantRenderHTML(markdown)
	if renderedHTML == "" {
		return nil
	}
	return map[string]interface{}{
		"type":        "render_chat",
		"turn_id":     strings.TrimSpace(turnID),
		"output_mode": normalizeTurnOutputMode(outputMode),
		"markdown":    strings.TrimSpace(markdown),
		"html":        renderedHTML,
	}
}

func assistantRenderCanvasCommand(turnID, outputMode, title, path, markdown string) map[string]interface{} {
	renderedHTML := assistantRenderHTML(markdown)
	if renderedHTML == "" {
		return nil
	}
	payload := map[string]interface{}{
		"type":        "render_canvas",
		"turn_id":     strings.TrimSpace(turnID),
		"output_mode": normalizeTurnOutputMode(outputMode),
		"kind":        "text_artifact",
		"title":       strings.TrimSpace(title),
		"path":        strings.TrimSpace(path),
		"text":        strings.TrimSpace(markdown),
		"html":        renderedHTML,
	}
	if payload["title"] == "" {
		delete(payload, "title")
	}
	if payload["path"] == "" {
		delete(payload, "path")
	}
	return payload
}
