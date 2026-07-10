package substitute

import (
	"strings"
	"testing"
)

func TestApply(t *testing.T) {
	t.Parallel()

	input := []byte("domain: ${DOMAIN}\nurl: https://${DOMAIN}/${PATH}\nempty: ${EMPTY}\n")
	values := map[string]string{
		"DOMAIN": "example.com",
		"PATH":   "api",
		"EMPTY":  "",
	}

	got, err := Apply(input, values)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	want := "domain: example.com\nurl: https://example.com/api\nempty: \n"
	if string(got) != want {
		t.Errorf("Apply() = %q, want %q", got, want)
	}
}

func TestApplyLeavesUnknownExpressionsUnchanged(t *testing.T) {
	t.Parallel()

	input := []byte("known: ${KNOWN}\nunknown: ${UNKNOWN}\nrepeated: ${UNKNOWN}\n")
	got, err := Apply(input, map[string]string{"KNOWN": "replaced"})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if want := "known: replaced\nunknown: ${UNKNOWN}\nrepeated: ${UNKNOWN}\n"; string(got) != want {
		t.Errorf("Apply() = %q, want %q", got, want)
	}
}

func TestApplyLeavesUnsupportedExpressionsUnchanged(t *testing.T) {
	t.Parallel()

	input := "plain $NAME, invalid ${NOT-A-NAME}, and ${VALID}"
	got, err := Apply([]byte(input), map[string]string{"VALID": "done"})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if want := strings.Replace(input, "${VALID}", "done", 1); string(got) != want {
		t.Errorf("Apply() = %q, want %q", got, want)
	}
}

func TestApplyLimitedRejectsExpandedOutput(t *testing.T) {
	t.Parallel()

	got, err := ApplyLimited([]byte("name: ${NAME}\n"), map[string]string{"NAME": "expanded"}, 10)
	if err == nil {
		t.Fatal("ApplyLimited() error = nil, want output-size error")
	}
	if got != nil {
		t.Errorf("ApplyLimited() output = %q, want nil", got)
	}
}
