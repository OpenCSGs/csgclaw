package api

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

var agentSessionDOMDroppedElements = map[string]struct{}{
	"base": {}, "canvas": {}, "embed": {}, "head": {}, "iframe": {}, "link": {},
	"meta": {}, "noembed": {}, "noframes": {}, "noscript": {}, "object": {},
	"plaintext": {}, "script": {}, "style": {}, "svg": {}, "template": {}, "xmp": {},
}

var agentSessionDOMAllowedElements = map[string]struct{}{
	"a": {}, "article": {}, "aside": {}, "b": {}, "blockquote": {}, "br": {},
	"button": {}, "code": {}, "dd": {}, "details": {}, "dialog": {}, "div": {},
	"dl": {}, "dt": {}, "em": {}, "fieldset": {}, "figcaption": {}, "figure": {},
	"footer": {}, "form": {}, "h1": {}, "h2": {}, "h3": {}, "h4": {}, "h5": {},
	"h6": {}, "header": {}, "hr": {}, "i": {}, "img": {}, "input": {}, "label": {},
	"legend": {}, "li": {}, "main": {}, "nav": {}, "ol": {}, "option": {}, "p": {},
	"pre": {}, "section": {}, "select": {}, "small": {}, "span": {}, "strong": {},
	"summary": {}, "table": {}, "tbody": {}, "td": {}, "textarea": {}, "tfoot": {},
	"th": {}, "thead": {}, "tr": {}, "u": {}, "ul": {},
}

var agentSessionDOMAllowedAttributes = map[string]struct{}{
	"alt": {}, "aria-checked": {}, "aria-current": {}, "aria-describedby": {},
	"aria-disabled": {}, "aria-expanded": {}, "aria-label": {}, "aria-labelledby": {},
	"aria-pressed": {}, "aria-selected": {}, "checked": {}, "colspan": {},
	"disabled": {}, "for": {}, "id": {}, "max": {}, "maxlength": {}, "min": {},
	"minlength": {}, "multiple": {}, "name": {}, "open": {}, "placeholder": {},
	"readonly": {}, "required": {}, "role": {}, "rowspan": {}, "selected": {},
	"step": {}, "title": {}, "type": {},
}

func sanitizeAgentSessionWebPageDOM(raw string) (string, error) {
	document, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return "", err
	}
	container := &html.Node{Type: html.ElementNode, Data: "div"}
	root := findAgentSessionDOMElement(document, "html")
	if root != nil {
		for _, sanitized := range sanitizeAgentSessionDOMNode(root) {
			container.AppendChild(sanitized)
		}
	} else {
		for child := document.FirstChild; child != nil; child = child.NextSibling {
			for _, sanitized := range sanitizeAgentSessionDOMNode(child) {
				container.AppendChild(sanitized)
			}
		}
	}

	var output bytes.Buffer
	for child := container.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&output, child); err != nil {
			return "", fmt.Errorf("render sanitized DOM: %w", err)
		}
	}
	return strings.TrimSpace(output.String()), nil
}

const agentSessionDOMTruncatedMarker = `<p>[page DOM truncated by CSGClaw]</p>`

func truncateAgentSessionSanitizedWebPageDOM(sanitized string, limit int) (string, error) {
	if limit <= 0 {
		return "", fmt.Errorf("sanitized DOM limit must be positive")
	}
	if len(sanitized) <= limit {
		return sanitized, nil
	}
	contentLimit := limit - len(agentSessionDOMTruncatedMarker) - 1
	if contentLimit <= 0 {
		return "", fmt.Errorf("sanitized DOM limit %d is too small", limit)
	}

	for contentLimit > 0 {
		prefix := truncateAgentSessionUTF8(sanitized, contentLimit)
		normalized, err := sanitizeAgentSessionWebPageDOM(prefix)
		if err != nil {
			return "", err
		}
		result := strings.TrimSpace(normalized) + "\n" + agentSessionDOMTruncatedMarker
		if len(result) <= limit {
			return result, nil
		}
		contentLimit -= len(result) - limit
	}
	return agentSessionDOMTruncatedMarker, nil
}

func truncateAgentSessionUTF8(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}

func sanitizeAgentSessionDOMNode(node *html.Node) []*html.Node {
	switch node.Type {
	case html.TextNode:
		if strings.TrimSpace(node.Data) == "" {
			return nil
		}
		return []*html.Node{{Type: html.TextNode, Data: node.Data}}
	case html.CommentNode, html.DoctypeNode:
		return nil
	case html.ElementNode:
		name := strings.ToLower(node.Data)
		if _, dropped := agentSessionDOMDroppedElements[name]; dropped || agentSessionDOMNodeHidden(node) {
			return nil
		}
		if name == "input" && agentSessionDOMInputSensitive(node) {
			return nil
		}
		if agentSessionDOMEditable(node) {
			return sanitizeAgentSessionDOMShallowNode(node, name)
		}

		children := sanitizeAgentSessionDOMChildren(node)
		if _, allowed := agentSessionDOMAllowedElements[name]; !allowed {
			return children
		}
		clean := &html.Node{Type: html.ElementNode, Data: name, Attr: sanitizeAgentSessionDOMAttributes(node)}
		for _, child := range children {
			clean.AppendChild(child)
		}
		return []*html.Node{clean}
	default:
		return nil
	}
}

func sanitizeAgentSessionDOMShallowNode(node *html.Node, name string) []*html.Node {
	if _, allowed := agentSessionDOMAllowedElements[name]; !allowed {
		return nil
	}
	clean := &html.Node{Type: html.ElementNode, Data: name, Attr: sanitizeAgentSessionDOMAttributes(node)}
	return []*html.Node{clean}
}

func sanitizeAgentSessionDOMChildren(node *html.Node) []*html.Node {
	var children []*html.Node
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		children = append(children, sanitizeAgentSessionDOMNode(child)...)
	}
	return children
}

func sanitizeAgentSessionDOMAttributes(node *html.Node) []html.Attribute {
	attributes := make([]html.Attribute, 0, len(node.Attr))
	for _, attribute := range node.Attr {
		name := strings.ToLower(attribute.Key)
		if name == "href" && strings.EqualFold(node.Data, "a") {
			if value := sanitizeAgentSessionDOMURL(attribute.Val); value != "" {
				attributes = append(attributes, html.Attribute{Key: "href", Val: value})
			}
			continue
		}
		if _, allowed := agentSessionDOMAllowedAttributes[name]; !allowed {
			continue
		}
		if name == "type" && strings.EqualFold(node.Data, "input") && agentSessionDOMInputSensitive(node) {
			continue
		}
		attributes = append(attributes, html.Attribute{Key: name, Val: attribute.Val})
	}
	return attributes
}

func sanitizeAgentSessionDOMURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if parsed.Scheme != "" && parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}

func agentSessionDOMNodeHidden(node *html.Node) bool {
	for _, attribute := range node.Attr {
		name := strings.ToLower(attribute.Key)
		value := strings.ToLower(strings.TrimSpace(attribute.Val))
		switch name {
		case "hidden":
			return true
		case "aria-hidden":
			if value == "true" {
				return true
			}
		case "style":
			compact := strings.NewReplacer(" ", "", "\t", "", "\r", "", "\n", "").Replace(value)
			if strings.Contains(compact, "display:none") || strings.Contains(compact, "visibility:hidden") {
				return true
			}
		}
	}
	return false
}

func agentSessionDOMInputSensitive(node *html.Node) bool {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, "type") {
			typeName := strings.ToLower(strings.TrimSpace(attribute.Val))
			return typeName == "hidden" || typeName == "password"
		}
	}
	return false
}

func agentSessionDOMEditable(node *html.Node) bool {
	if strings.EqualFold(node.Data, "textarea") {
		return true
	}
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, "contenteditable") {
			value := strings.ToLower(strings.TrimSpace(attribute.Val))
			return value == "" || value == "true" || value == "plaintext-only"
		}
	}
	return false
}

func findAgentSessionDOMElement(node *html.Node, name string) *html.Node {
	if node.Type == html.ElementNode && strings.EqualFold(node.Data, name) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findAgentSessionDOMElement(child, name); found != nil {
			return found
		}
	}
	return nil
}
