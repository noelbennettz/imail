package match

import (
	"github.com/dlclark/regexp2"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/utils/context"
	log "github.com/sirupsen/logrus"
)

type RegexMatch struct {
	Rule  string
	Field string
}

func NewRegexMatch(field, rule string) *RegexMatch {
	return &RegexMatch{
		Rule:  rule,
		Field: field,
	}
}

func (r *RegexMatch) Match(ctx *context.Context, email *parsemail.Email) bool {
	content := getFieldContent(r.Field, email)
	re := regexp2.MustCompile(r.Rule, 0)
	match, err := re.MatchString(content)

	if err != nil {
		log.WithContext(ctx).Errorf("rule regex error %v", err)
	}

	return match
}
