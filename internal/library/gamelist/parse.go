package gamelist

import (
	"bytes"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	xmlNamespace   = "http://www.w3.org/XML/1998/namespace"
	xmlnsNamespace = "http://www.w3.org/2000/xmlns/"
	utf8BOM        = "\xef\xbb\xbf"
)

// element is the parser's source-span index for one XML element. All offsets
// are byte offsets into the exact source supplied to Parse.
type element struct {
	qname QName
	attrs []Attribute
	scope map[string]string

	start       int
	startTagEnd int
	endTagStart int
	end         int

	selfClosing bool
	hasCDATA    bool

	contentComplex bool
	text           []byte
	children       []*element
}

func isSelfClosingTag(data []byte, start, end int) bool {
	return end >= start+2 && bytes.HasSuffix(data[start:end], []byte("/>"))
}

func (e *element) complex() bool {
	return e.contentComplex || e.hasCDATA
}

// Parse validates and indexes data as a gamelist.xml document. systemDir is
// used only for lexical ROM path classification; Parse never accesses the
// filesystem and never writes anything.
func Parse(systemDir string, data []byte) (*Document, error) {
	if len(data) == 0 {
		return nil, ErrEmpty
	}
	if hasNonUTF8BOM(data) || bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return nil, fmt.Errorf("%w: only UTF-8 and UTF-8 BOM are supported", ErrUnsupportedEncoding)
	}
	if err := validateDeclaredEncoding(data); err != nil {
		return nil, err
	}

	root, err := parseXML(data)
	if err != nil {
		return nil, err
	}
	if root.qname.Local != "gameList" {
		return nil, fmt.Errorf("%w: got %q", ErrRoot, root.qname.String())
	}

	if systemDir != "" {
		absolute, absErr := filepath.Abs(systemDir)
		if absErr != nil {
			return nil, fmt.Errorf("gamelist: resolve system directory: %w", absErr)
		}
		systemDir = absolute
	}

	doc := &Document{
		SystemDir:      systemDir,
		Root:           root.qname,
		RootAttributes: cloneAttributes(root.attrs),
		SourceHash:     sha256.Sum256(data),
		source:         append([]byte(nil), data...),
		ops:            defaultSaveOperations(),
	}

	for _, child := range root.children {
		if child.qname.Namespace != root.qname.Namespace {
			doc.Other = append(doc.Other, RawTopLevel{
				Name: child.qname,
				Raw:  append([]byte(nil), data[child.start:child.end]...),
			})
			continue
		}
		switch child.qname.Local {
		case "game":
			doc.addEntry(EntryGame, child)
		case "folder":
			doc.addEntry(EntryFolder, child)
		default:
			doc.Other = append(doc.Other, RawTopLevel{
				Name: child.qname,
				Raw:  append([]byte(nil), data[child.start:child.end]...),
			})
		}
	}
	return doc, nil
}

func (d *Document) addEntry(kind EntryKind, el *element) {
	index := len(d.Entries)
	entry := Entry{
		Kind:       kind,
		Index:      index,
		Name:       el.qname,
		Attributes: cloneAttributes(el.attrs),
	}
	ref := entryRef{index: index, el: el}
	ordinals := make(map[string]int)
	pathSet := false

	for _, child := range el.children {
		key := fieldKey(child.qname)
		ordinal := ordinals[key]
		ordinals[key]++

		field := Field{
			Name:       child.qname,
			Value:      string(child.text),
			Attributes: cloneAttributes(child.attrs),
			Ordinal:    ordinal,
			Complex:    child.complex(),
		}
		fieldIndex := len(entry.Fields)
		entry.Fields = append(entry.Fields, field)
		ref.fields = append(ref.fields, fieldRef{
			name:          child.qname,
			ordinal:       ordinal,
			fieldIndex:    fieldIndex,
			el:            child,
			originalValue: field.Value,
		})

		if !pathSet && child.qname.Namespace == el.qname.Namespace && child.qname.Local == "path" {
			entry.Path = classifyPath(d.SystemDir, field.Value)
			pathSet = true
		}
	}

	d.Entries = append(d.Entries, entry)
	d.refs = append(d.refs, ref)
}

func fieldKey(name QName) string {
	return name.Namespace + "\x00" + name.Local
}

func cloneAttributes(attrs []Attribute) []Attribute {
	if len(attrs) == 0 {
		return nil
	}
	out := make([]Attribute, len(attrs))
	copy(out, attrs)
	return out
}

func hasNonUTF8BOM(data []byte) bool {
	if len(data) >= 2 {
		if data[0] == 0xff && data[1] == 0xfe {
			return true
		}
		if data[0] == 0xfe && data[1] == 0xff {
			return true
		}
	}
	if len(data) >= 4 {
		if data[0] == 0x00 && data[1] == 0x00 && data[2] == 0xfe && data[3] == 0xff {
			return true
		}
	}
	return false
}

func parseXML(data []byte) (*element, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true

	var stack []*element
	var root *element
	var previous int64
	var xmlDeclarationSeen bool
	var bomHandled bool
	declarationOffset := int64(0)
	if bytes.HasPrefix(data, []byte(utf8BOM)) {
		declarationOffset = int64(len(utf8BOM))
	}

	for {
		token, err := decoder.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidXML, err)
		}
		offset := decoder.InputOffset()
		if offset < previous || offset > int64(len(data)) {
			return nil, fmt.Errorf("%w: non-monotonic XML offset", ErrInvalidXML)
		}

		switch typed := token.(type) {
		case xml.CharData:
			if len(stack) == 0 {
				text := []byte(typed)
				if !bomHandled && previous == 0 && bytes.HasPrefix(text, []byte(utf8BOM)) {
					text = text[len(utf8BOM):]
					bomHandled = true
				}
				if len(bytes.TrimSpace(text)) != 0 {
					return nil, fmt.Errorf("%w: character data outside the root element", ErrInvalidXML)
				}
				break
			}
			current := stack[len(stack)-1]
			current.text = append(current.text, typed...)

		case xml.StartElement:
			if root != nil && len(stack) == 0 {
				return nil, fmt.Errorf("%w: multiple root elements", ErrInvalidXML)
			}
			if err := validateAttributes(typed.Attr); err != nil {
				return nil, err
			}

			start := int(previous)
			startTagEnd := int(offset)
			if start < 0 || startTagEnd < start || startTagEnd > len(data) {
				return nil, fmt.Errorf("%w: invalid start-tag span", ErrInvalidXML)
			}

			parentScope := map[string]string{"": "", "xml": xmlNamespace, "xmlns": xmlnsNamespace}
			if len(stack) > 0 {
				parentScope = cloneScope(stack[len(stack)-1].scope)
			}
			scope, err := applyNamespaceDeclarations(parentScope, typed.Attr)
			if err != nil {
				return nil, err
			}
			qname, err := resolveElementName(typed.Name, scope)
			if err != nil {
				return nil, err
			}
			attrs, err := resolveAttributes(typed.Attr, scope)
			if err != nil {
				return nil, err
			}

			el := &element{
				qname:       qname,
				attrs:       attrs,
				scope:       scope,
				start:       start,
				startTagEnd: startTagEnd,
			}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.contentComplex = true
				parent.children = append(parent.children, el)
			}
			if root == nil {
				root = el
			}
			stack = append(stack, el)

		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("%w: end element without start", ErrInvalidXML)
			}
			current := stack[len(stack)-1]
			endName, err := resolveElementName(typed.Name, current.scope)
			if err != nil {
				return nil, err
			}
			// XML end tags must match the original QName, not merely the
			// expanded {namespace URI}local name. Prefixes are part of the
			// source document and are preserved by this package's splice
			// model, so a different prefix must be rejected even when it is
			// bound to the same URI.
			if typed.Name.Space != current.qname.Prefix || typed.Name.Local != current.qname.Local {
				return nil, fmt.Errorf("%w: mismatched end element </%s> for <%s>", ErrInvalidXML, typed.Name.Local, current.qname.String())
			}
			if endName.Namespace != current.qname.Namespace {
				return nil, fmt.Errorf("%w: end element namespace changed for <%s>", ErrInvalidXML, current.qname.String())
			}
			stack = stack[:len(stack)-1]

			endTagStart := int(previous)
			end := int(offset)
			if endTagStart < current.startTagEnd || end < endTagStart || end > len(data) {
				return nil, fmt.Errorf("%w: invalid element span", ErrInvalidXML)
			}
			current.endTagStart = endTagStart
			current.end = end
			current.selfClosing = endTagStart == current.startTagEnd && isSelfClosingTag(data, current.start, current.startTagEnd)
			if current.selfClosing && end != endTagStart {
				return nil, fmt.Errorf("%w: invalid self-closing span", ErrInvalidXML)
			}
			if endTagStart > current.startTagEnd {
				current.hasCDATA = bytes.Contains(data[current.startTagEnd:endTagStart], []byte("<![CDATA["))
			}

		case xml.Comment:
			if len(stack) > 0 {
				stack[len(stack)-1].contentComplex = true
			}

		case xml.ProcInst:
			if typed.Target == "xml" {
				if xmlDeclarationSeen || root != nil || len(stack) > 0 || previous != declarationOffset {
					return nil, fmt.Errorf("%w: misplaced XML declaration", ErrInvalidXML)
				}
				xmlDeclarationSeen = true
				if err := validateXMLDeclaration(typed.Inst); err != nil {
					return nil, err
				}
			} else if len(stack) > 0 {
				stack[len(stack)-1].contentComplex = true
			}

		case xml.Directive:
			if len(stack) > 0 {
				stack[len(stack)-1].contentComplex = true
			}
		}

		previous = offset
	}

	if root == nil {
		return nil, fmt.Errorf("%w: no root element", ErrInvalidXML)
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("%w: unclosed element <%s>", ErrInvalidXML, stack[len(stack)-1].qname.String())
	}
	return root, nil
}

func validateDeclaredEncoding(data []byte) error {
	start := 0
	if bytes.HasPrefix(data, []byte(utf8BOM)) {
		start = len(utf8BOM)
	}
	if !bytes.HasPrefix(data[start:], []byte("<?xml")) {
		return nil
	}
	end := bytes.Index(data[start:], []byte("?>"))
	if end < 0 {
		return nil
	}
	declaration := string(data[start : start+end])
	lower := strings.ToLower(declaration)
	index := strings.Index(lower, "encoding")
	if index < 0 {
		return nil
	}
	rest := strings.TrimSpace(declaration[index+len("encoding"):])
	if !strings.HasPrefix(rest, "=") {
		return nil
	}
	rest = strings.TrimSpace(rest[1:])
	if len(rest) < 2 || rest[0] != '"' && rest[0] != '\'' {
		return nil
	}
	quote := rest[0]
	endQuote := strings.IndexByte(rest[1:], quote)
	if endQuote < 0 {
		return nil
	}
	encoding := rest[1 : 1+endQuote]
	if !strings.EqualFold(encoding, "utf-8") && !strings.EqualFold(encoding, "utf8") {
		return fmt.Errorf("%w: declared encoding %q", ErrUnsupportedEncoding, encoding)
	}
	return nil
}

func validateXMLDeclaration(inst []byte) error {
	var declaration struct {
		Encoding string `xml:"encoding,attr"`
	}
	if err := xml.Unmarshal([]byte("<x "+string(inst)+"/>"), &declaration); err != nil {
		return fmt.Errorf("%w: malformed XML declaration: %v", ErrInvalidXML, err)
	}
	if declaration.Encoding != "" && !strings.EqualFold(declaration.Encoding, "utf-8") {
		return fmt.Errorf("%w: declared encoding %q", ErrUnsupportedEncoding, declaration.Encoding)
	}
	return nil
}

func validateAttributes(attrs []xml.Attr) error {
	seen := make(map[string]struct{}, len(attrs))
	for _, attr := range attrs {
		key := attr.Name.Space + "\x00" + attr.Name.Local
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%w: duplicate attribute %q", ErrInvalidXML, attr.Name.Local)
		}
		seen[key] = struct{}{}
		if attr.Name.Space == "xmlns" && attr.Name.Local == "xmlns" {
			return fmt.Errorf("%w: invalid xmlns prefix declaration", ErrInvalidXML)
		}
		if attr.Name.Space == "xmlns" && attr.Name.Local == "xml" && attr.Value != xmlNamespace {
			return fmt.Errorf("%w: invalid xml namespace binding", ErrInvalidXML)
		}
	}
	return nil
}

func cloneScope(scope map[string]string) map[string]string {
	out := make(map[string]string, len(scope)+1)
	for key, value := range scope {
		out[key] = value
	}
	return out
}

func applyNamespaceDeclarations(scope map[string]string, attrs []xml.Attr) (map[string]string, error) {
	for _, attr := range attrs {
		switch {
		case attr.Name.Space == "" && attr.Name.Local == "xmlns":
			scope[""] = attr.Value
		case attr.Name.Space == "xmlns":
			scope[attr.Name.Local] = attr.Value
		}
	}
	return scope, nil
}

func resolveElementName(name xml.Name, scope map[string]string) (QName, error) {
	if name.Space == "xmlns" {
		return QName{}, fmt.Errorf("%w: element prefix xmlns is reserved", ErrInvalidXML)
	}
	namespace, err := resolveNamespace(name.Space, scope)
	if err != nil {
		return QName{}, err
	}
	return QName{Prefix: name.Space, Namespace: namespace, Local: name.Local}, nil
}

func resolveAttributes(attrs []xml.Attr, scope map[string]string) ([]Attribute, error) {
	out := make([]Attribute, 0, len(attrs))
	seen := make(map[string]struct{}, len(attrs))
	for _, attr := range attrs {
		var name QName
		switch {
		case attr.Name.Space == "" && attr.Name.Local == "xmlns":
			name = QName{Prefix: "", Namespace: xmlnsNamespace, Local: "xmlns"}
		case attr.Name.Space == "xmlns":
			name = QName{Prefix: "xmlns", Namespace: xmlnsNamespace, Local: attr.Name.Local}
		case attr.Name.Space == "":
			name = QName{Local: attr.Name.Local}
		default:
			namespace, err := resolveNamespace(attr.Name.Space, scope)
			if err != nil {
				return nil, err
			}
			name = QName{Prefix: attr.Name.Space, Namespace: namespace, Local: attr.Name.Local}
		}
		key := name.Namespace + "\x00" + name.Local
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("%w: duplicate expanded attribute %q", ErrInvalidXML, name.String())
		}
		seen[key] = struct{}{}
		out = append(out, Attribute{Name: name, Value: attr.Value})
	}
	return out, nil
}

func resolveNamespace(prefix string, scope map[string]string) (string, error) {
	switch prefix {
	case "":
		return scope[""], nil
	case "xml":
		return xmlNamespace, nil
	case "xmlns":
		return xmlnsNamespace, nil
	}
	namespace, ok := scope[prefix]
	if !ok {
		return "", fmt.Errorf("%w: unbound namespace prefix %q", ErrInvalidXML, prefix)
	}
	return namespace, nil
}
