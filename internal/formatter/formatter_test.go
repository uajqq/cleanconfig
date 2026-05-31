package formatter

import "testing"

func TestIndentsNestedConfigObjSectionsAndValues(t *testing.T) {
	source := `debug=0
 [Station]
location='WeeWX station'
altitude = 0, 'foot'    # Choose unit
[[Nested]]
enabled=false
`
	expected := `debug = 0
[Station]
    location = "WeeWX station"
    altitude = 0, "foot"  # Choose unit
    [[Nested]]
        enabled = false
`
	assertFormatted(t, source, DefaultOptions(), expected)
}

func TestKeepsTopLevelPreambleCommentsAtTopLevel(t *testing.T) {
	source := `[Station]
location = home

    ##############################################################################
    # This section is for uploading data to Internet sites.

    [StdRESTful]
[[AWEKAS]]
enable=false
`
	expected := `[Station]
    location = home

##############################################################################
# This section is for uploading data to Internet sites.

[StdRESTful]
    [[AWEKAS]]
        enable = false
`
	assertFormatted(t, source, DefaultOptions(), expected)
}

func TestAlignsContiguousAssignmentBlocks(t *testing.T) {
	source := `[Extras]
theme="auto"
theme_toggle_enabled=1
googleAnalyticsId = ''
# break
radar_width=650
radar_height =360
`
	expected := `[Extras]
    theme                = "auto"
    theme_toggle_enabled = 1
    googleAnalyticsId    = ""
    # break
    radar_width  = 650
    radar_height = 360
`
	opts := DefaultOptions()
	opts.AlignEquals = true
	assertFormatted(t, source, opts, expected)
}

func TestPreservesQuotedKeysAndSplitsCommentsOutsideQuotes(t *testing.T) {
	source := `[Almanac]
[[TZ]]
"name(LAT)"='solar # time' # local apparent solar time
`
	expected := `[Almanac]
    [[TZ]]
        "name(LAT)" = "solar # time"  # local apparent solar time
`
	assertFormatted(t, source, DefaultOptions(), expected)
}

func TestPreserveQuoteStyleLeavesValuesAlone(t *testing.T) {
	source := `[Extras]
theme='auto'
items = 'a', "b"
`
	expected := `[Extras]
    theme = 'auto'
    items = 'a', "b"
`
	opts := DefaultOptions()
	opts.QuoteStyle = QuotePreserve
	assertFormatted(t, source, opts, expected)
}

func TestAutoQuoteStyleUsesSingleQuotesToAvoidEscapingDoubleQuotes(t *testing.T) {
	source := `[Extras]
html = "Use \"quoted\" words"
plain = 'normal'
`
	expected := `[Extras]
    html = 'Use "quoted" words'
    plain = "normal"
`
	opts := DefaultOptions()
	opts.QuoteStyle = QuoteAuto
	assertFormatted(t, source, opts, expected)
}

func TestIdempotent(t *testing.T) {
	source := `# WEEWX CONFIGURATION FILE

[StdReport]
    SKIN_ROOT = skins

    [[SeasonsReport]]
        skin = Seasons
        enable = true
`
	first, err := FormatConfig(source, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	second, err := FormatConfig(first, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("expected idempotent output\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestCollapsesRedundantBlankLines(t *testing.T) {
	source := `

[Station]
location = home


altitude = 0, foot



[StdRESTful]
[[AWEKAS]]
enable=false


`
	expected := `[Station]
    location = home

    altitude = 0, foot

[StdRESTful]
    [[AWEKAS]]
        enable = false
`
	assertFormatted(t, source, DefaultOptions(), expected)
}

func TestPreservesBlankLinesInsideMultilineValues(t *testing.T) {
	source := `[Station]
message = """
first


second
"""


location = home
`
	expected := `[Station]
    message = """
first


second
"""

    location = home
`
	assertFormatted(t, source, DefaultOptions(), expected)
}

func TestEmptyInputStaysEmpty(t *testing.T) {
	assertFormatted(t, "", DefaultOptions(), "")
}

func assertFormatted(t *testing.T, source string, opts Options, expected string) {
	t.Helper()

	actual, err := FormatConfig(source, opts)
	if err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("formatted output mismatch\nexpected:\n%s\nactual:\n%s", expected, actual)
	}
}
