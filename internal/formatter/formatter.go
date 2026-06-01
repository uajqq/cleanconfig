package formatter

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var sectionPattern = regexp.MustCompile(`^(\[+)\s*(.*?)\s*(\]+)(\s*(?:#.*)?)$`)

type QuoteStyle string

const (
	QuoteDouble   QuoteStyle = "double"
	QuoteSingle   QuoteStyle = "single"
	QuoteAuto     QuoteStyle = "auto"
	QuotePreserve QuoteStyle = "preserve"
)

type QuotePolicy string

const (
	QuoteExisting QuotePolicy = "existing"
	QuoteAll      QuotePolicy = "all"
)

type Options struct {
	IndentWidth  int
	AlignEquals  bool
	QuoteStyle   QuoteStyle
	QuotePolicy  QuotePolicy
	FinalNewline bool
}

func DefaultOptions() Options {
	return Options{
		IndentWidth:  4,
		QuoteStyle:   QuoteDouble,
		QuotePolicy:  QuoteExisting,
		FinalNewline: true,
	}
}

func (o Options) Validate() error {
	if o.IndentWidth < 0 {
		return errors.New("indent width must be non-negative")
	}
	switch o.QuoteStyle {
	case QuoteDouble, QuoteSingle, QuoteAuto, QuotePreserve:
	default:
		return errors.New("quote style must be double, single, auto, or preserve")
	}
	switch o.QuotePolicy {
	case QuoteExisting, QuoteAll:
	default:
		return errors.New("quote policy must be existing or all")
	}
	return nil
}

type assignment struct {
	key       string
	value     string
	comment   string
	multiline bool
}

type parsedLine struct {
	kind           string
	text           string
	level          int
	sectionLevel   int
	sectionName    string
	sectionComment string
	assignment     *assignment
}

type renderedLine struct {
	text         string
	kind         string
	sectionLevel int
	assignment   *assignment
}

func FormatConfig(text string, opts Options) (string, error) {
	if err := opts.Validate(); err != nil {
		return "", err
	}

	lines := parseLines(text)
	rendered := renderLines(lines, opts)

	if opts.AlignEquals {
		rendered = alignAssignmentBlocks(rendered)
	}

	parts := make([]string, len(rendered))
	for i, line := range rendered {
		parts[i] = line.text
	}
	result := strings.Join(parts, "\n")
	if opts.FinalNewline {
		if result == "" {
			return "", nil
		}
		result = strings.TrimRight(result, "\n") + "\n"
	}
	return result, nil
}

func parseLines(text string) []parsedLine {
	bodies := splitBodies(text)
	lines := make([]parsedLine, 0, len(bodies))
	currentLevel := 0
	inMultiline := false
	multilineQuote := ""

	for _, body := range bodies {
		if inMultiline {
			lines = append(lines, parsedLine{kind: "multiline", text: body, level: currentLevel})
			if containsUnescaped(body, multilineQuote) {
				inMultiline = false
				multilineQuote = ""
			}
			continue
		}

		if sections, ok := splitAdjacentSectionBodies(body); ok {
			for _, section := range sections {
				sectionLevel, name, comment, _ := parseSection(strings.TrimSpace(section))
				lines = append(lines, parsedLine{
					kind:           "section",
					text:           section,
					level:          currentLevel,
					sectionLevel:   sectionLevel,
					sectionName:    name,
					sectionComment: comment,
				})
				currentLevel = sectionLevel
			}
			continue
		}

		stripped := strings.TrimSpace(body)
		if stripped == "" {
			lines = append(lines, parsedLine{kind: "blank", level: currentLevel})
			continue
		}

		if sectionLevel, name, comment, ok := parseSection(stripped); ok {
			lines = append(lines, parsedLine{
				kind:           "section",
				text:           body,
				level:          currentLevel,
				sectionLevel:   sectionLevel,
				sectionName:    name,
				sectionComment: comment,
			})
			currentLevel = sectionLevel
			continue
		}

		if strings.HasPrefix(stripped, "#") {
			lines = append(lines, parsedLine{kind: "comment", text: body, level: currentLevel})
			continue
		}

		if assignment, ok := parseAssignment(stripped); ok {
			lines = append(lines, parsedLine{
				kind:       "assignment",
				text:       body,
				level:      currentLevel,
				assignment: &assignment,
			})
			if assignment.multiline {
				inMultiline = true
				if strings.Contains(assignment.value, `"""`) {
					multilineQuote = `"""`
				} else {
					multilineQuote = `'''`
				}
			}
			continue
		}

		lines = append(lines, parsedLine{kind: "other", text: body, level: currentLevel})
	}

	return lines
}

func splitBodies(text string) []string {
	if text == "" {
		return nil
	}
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	if strings.HasSuffix(normalized, "\n") {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func splitAdjacentSectionBodies(body string) ([]string, bool) {
	stripped := strings.TrimSpace(body)
	if stripped == "" || stripped[0] != '[' {
		return nil, false
	}

	sections := []string{}
	index := 0
	for index < len(stripped) {
		index = skipSpace(stripped, index)
		if index >= len(stripped) {
			break
		}
		if stripped[index] == '#' {
			if len(sections) == 0 {
				return nil, false
			}
			sections[len(sections)-1] += "  " + strings.TrimSpace(stripped[index:])
			break
		}
		if stripped[index] != '[' {
			return nil, false
		}

		start := index
		openingStart := index
		for index < len(stripped) && stripped[index] == '[' {
			index++
		}
		level := index - openingStart

		nameStart := index
		for index < len(stripped) && stripped[index] != '[' && stripped[index] != ']' {
			index++
		}
		if index >= len(stripped) || stripped[index] != ']' {
			return nil, false
		}
		if strings.TrimSpace(stripped[nameStart:index]) == "" {
			return nil, false
		}

		closingStart := index
		for index < len(stripped) && stripped[index] == ']' {
			index++
		}
		if index-closingStart != level {
			return nil, false
		}

		sections = append(sections, strings.TrimSpace(stripped[start:index]))
	}

	if len(sections) < 2 {
		return nil, false
	}
	return sections, true
}

func skipSpace(text string, index int) int {
	for index < len(text) {
		char, size := utf8.DecodeRuneInString(text[index:])
		if !unicode.IsSpace(char) {
			return index
		}
		index += size
	}
	return index
}

func parseSection(stripped string) (int, string, string, bool) {
	matches := sectionPattern.FindStringSubmatch(stripped)
	if matches == nil {
		return 0, "", "", false
	}

	opening := matches[1]
	name := matches[2]
	closing := matches[3]
	comment := strings.TrimSpace(matches[4])
	if len(opening) != len(closing) {
		return 0, "", "", false
	}
	if strings.ContainsAny(name, "[]") {
		return 0, "", "", false
	}

	sectionName := strings.TrimSpace(name)
	if sectionName == "" {
		return 0, "", "", false
	}
	return len(opening), sectionName, comment, true
}

func parseAssignment(stripped string) (assignment, bool) {
	index := findUnquoted(stripped, '=')
	if index < 1 {
		return assignment{}, false
	}

	key := strings.TrimSpace(stripped[:index])
	if key == "" {
		return assignment{}, false
	}

	rawValue := strings.TrimLeftFunc(stripped[index+1:], unicode.IsSpace)
	value, comment := splitValueComment(rawValue)
	return assignment{
		key:       key,
		value:     value,
		comment:   comment,
		multiline: startsMultilineValue(value),
	}, true
}

func splitValueComment(valueText string) (string, string) {
	inSingle := false
	inDouble := false
	escaped := false

	for index, char := range valueText {
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' && (inSingle || inDouble) {
			escaped = true
			continue
		}
		if char == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if char == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if char == '#' && !inSingle && !inDouble {
			return trimRightSpace(valueText[:index]), strings.TrimRightFunc(valueText[index:], unicode.IsSpace)
		}
	}

	return trimRightSpace(valueText), ""
}

func renderLines(lines []parsedLine, opts Options) []renderedLine {
	rendered := make([]renderedLine, 0, len(lines))
	preSectionLevels := findPreSectionLevels(lines)
	previousBlank := false

	appendRendered := func(line renderedLine) {
		rendered = append(rendered, line)
		previousBlank = false
	}

	for index, line := range lines {
		switch {
		case line.kind == "blank":
			if len(rendered) == 0 || previousBlank {
				continue
			}
			rendered = append(rendered, renderedLine{})
			previousBlank = true

		case line.kind == "section":
			indent := indent(line.sectionLevel-1, opts)
			body := indent + strings.Repeat("[", line.sectionLevel) + line.sectionName + strings.Repeat("]", line.sectionLevel)
			if line.sectionComment != "" {
				body += "  " + line.sectionComment
			}
			if shouldSeparateAdjacentSection(rendered, previousBlank, line.sectionLevel) {
				rendered = append(rendered, renderedLine{})
				previousBlank = true
			}
			appendRendered(renderedLine{text: body, kind: "section", sectionLevel: line.sectionLevel})

		case line.kind == "assignment" && line.assignment != nil:
			indent := indent(line.level, opts)
			body := formatAssignment(*line.assignment, opts)
			appendRendered(renderedLine{text: indent + body, assignment: line.assignment})

		case line.kind == "multiline":
			appendRendered(renderedLine{text: trimRightSpace(line.text)})

		default:
			level, ok := preSectionLevels[index]
			if !ok {
				level = line.level
			}
			appendRendered(renderedLine{text: indent(level, opts) + strings.TrimSpace(line.text)})
		}
	}
	if previousBlank {
		rendered = rendered[:len(rendered)-1]
	}

	return rendered
}

func shouldSeparateAdjacentSection(rendered []renderedLine, previousBlank bool, sectionLevel int) bool {
	if previousBlank || len(rendered) == 0 {
		return false
	}

	previous := rendered[len(rendered)-1]
	return previous.kind == "section" && sectionLevel <= previous.sectionLevel
}

func findPreSectionLevels(lines []parsedLine) map[int]int {
	levels := make(map[int]int)

	for index, line := range lines {
		if line.kind != "section" {
			continue
		}

		preambleLevel := line.sectionLevel - 1
		if preambleLevel < 0 {
			preambleLevel = 0
		}
		for cursor := index - 1; cursor >= 0; cursor-- {
			kind := lines[cursor].kind
			if kind != "blank" && kind != "comment" && kind != "other" {
				break
			}
			if kind != "blank" {
				levels[cursor] = preambleLevel
			}
		}
	}

	return levels
}

func formatAssignment(item assignment, opts Options) string {
	value := item.value
	if !item.multiline && opts.QuoteStyle != QuotePreserve {
		value = formatValue(item.value, opts)
	}

	result := item.key + " = " + value
	if item.comment != "" {
		result += "  " + item.comment
	}
	return result
}

func formatValue(value string, opts Options) string {
	if value == "" {
		return value
	}

	items := splitListItems(value)
	if len(items) == 1 {
		return formatValueItem(items[0], opts)
	}

	formatted := make([]string, len(items))
	for i, item := range items {
		formatted[i] = formatValueItem(item, opts)
	}
	return strings.Join(formatted, ", ")
}

func splitListItems(value string) []string {
	items := []string{}
	start := 0
	inSingle := false
	inDouble := false
	escaped := false

	for index, char := range value {
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' && (inSingle || inDouble) {
			escaped = true
			continue
		}
		if char == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if char == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if char == ',' && !inSingle && !inDouble {
			items = append(items, strings.TrimSpace(value[start:index]))
			start = index + len(string(char))
		}
	}

	items = append(items, strings.TrimSpace(value[start:]))
	return items
}

func formatValueItem(item string, opts Options) string {
	if item == "" {
		return item
	}

	oldQuote, inner, quoted := quotedItem(item)
	if !quoted {
		if opts.QuotePolicy == QuoteAll {
			return quoteItem(item, opts.QuoteStyle, 0)
		}
		return item
	}

	return quoteItem(inner, opts.QuoteStyle, oldQuote)
}

func quotedItem(item string) (rune, string, bool) {
	if len(item) < 2 {
		return 0, "", false
	}

	quote := rune(item[0])
	if quote != '\'' && quote != '"' {
		return 0, "", false
	}
	if rune(item[len(item)-1]) != quote {
		return 0, "", false
	}
	if strings.HasPrefix(item, strings.Repeat(string(quote), 3)) {
		return 0, "", false
	}

	return quote, item[1 : len(item)-1], true
}

func quoteItem(inner string, style QuoteStyle, oldQuote rune) string {
	content := inner
	if oldQuote != 0 {
		content = unescapeQuote(inner, oldQuote)
	}
	quote := chooseQuote(content, style)
	escaped := escapeQuote(content, quote)
	return string(quote) + escaped + string(quote)
}

func chooseQuote(content string, style QuoteStyle) rune {
	switch style {
	case QuoteSingle:
		return '\''
	case QuoteAuto:
		if strings.Contains(content, `"`) && !strings.Contains(content, `'`) {
			return '\''
		}
		return '"'
	default:
		return '"'
	}
}

func unescapeQuote(text string, quote rune) string {
	var builder strings.Builder
	escaped := false

	for _, char := range text {
		if escaped {
			if char == quote {
				builder.WriteRune(char)
			} else {
				builder.WriteRune('\\')
				builder.WriteRune(char)
			}
			escaped = false
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		builder.WriteRune(char)
	}

	if escaped {
		builder.WriteRune('\\')
	}
	return builder.String()
}

func escapeQuote(text string, quote rune) string {
	var builder strings.Builder
	escaped := false

	for _, char := range text {
		if escaped {
			builder.WriteRune('\\')
			builder.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		if char == quote {
			builder.WriteRune('\\')
			builder.WriteRune(char)
			continue
		}
		builder.WriteRune(char)
	}

	if escaped {
		builder.WriteRune('\\')
	}
	return builder.String()
}

func alignAssignmentBlocks(rendered []renderedLine) []renderedLine {
	aligned := make([]renderedLine, len(rendered))
	copy(aligned, rendered)
	block := []int{}

	flush := func() {
		if len(block) < 2 {
			block = block[:0]
			return
		}

		maxKeyWidth := 0
		for _, index := range block {
			line := aligned[index].text
			eq := findUnquoted(line, '=')
			if eq < 0 {
				continue
			}
			key := strings.TrimSpace(line[:eq])
			if len(key) > maxKeyWidth {
				maxKeyWidth = len(key)
			}
		}

		for _, index := range block {
			line := aligned[index].text
			eq := findUnquoted(line, '=')
			if eq < 0 {
				continue
			}
			left := line[:eq]
			right := line[eq+1:]
			indentWidth := len(left) - len(strings.TrimLeftFunc(left, unicode.IsSpace))
			key := strings.TrimSpace(left)
			aligned[index].text = strings.Repeat(" ", indentWidth) + padRight(key, maxKeyWidth) + " = " + strings.TrimLeftFunc(right, unicode.IsSpace)
		}
		block = block[:0]
	}

	for index, line := range aligned {
		if line.assignment == nil {
			flush()
			continue
		}
		block = append(block, index)
	}
	flush()

	return aligned
}

func findUnquoted(text string, needle rune) int {
	inSingle := false
	inDouble := false
	escaped := false

	for index, char := range text {
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' && (inSingle || inDouble) {
			escaped = true
			continue
		}
		if char == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if char == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if char == needle && !inSingle && !inDouble {
			return index
		}
	}

	return -1
}

func startsMultilineValue(value string) bool {
	for _, quote := range []string{`"""`, `'''`} {
		if strings.Contains(value, quote) && countUnescaped(value, quote)%2 == 1 {
			return true
		}
	}
	return false
}

func countUnescaped(text string, needle string) int {
	count := 0
	start := 0
	for {
		index := strings.Index(text[start:], needle)
		if index < 0 {
			return count
		}
		index += start
		if index == 0 || text[index-1] != '\\' {
			count++
		}
		start = index + len(needle)
	}
}

func containsUnescaped(text string, needle string) bool {
	return countUnescaped(text, needle) > 0
}

func indent(level int, opts Options) string {
	if level < 0 {
		level = 0
	}
	return strings.Repeat(" ", level*opts.IndentWidth)
}

func trimRightSpace(text string) string {
	return strings.TrimRightFunc(text, unicode.IsSpace)
}

func padRight(text string, width int) string {
	if len(text) >= width {
		return text
	}
	return text + strings.Repeat(" ", width-len(text))
}
