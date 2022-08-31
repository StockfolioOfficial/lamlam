package config

import (
	"strings"
)

const (
	Version1 = "1"
)

type Config struct {
	Version    string     `yaml:"version"`
	LambdaList LambdaList `yaml:"lambda"`
}

type LambdaList []Lambda

//TODO: need error return
func (list LambdaList) Patterns() (res []string) {
	res = make([]string, 0, len(list))
	for _, item := range list {

		pattern := item.Pattern()
		if pattern == "" {
			//TODO: error
			continue
		}

		res = append(res, pattern)
	}

	return
}

func (list LambdaList) ToMap() map[string]*Lambda {
	res := make(map[string]*Lambda)

	for i := range list {
		l := &list[i]
		res[l.Pattern()] = l
	}

	return res
}

type Lambda struct {
	Type       string `yaml:"type"`
	LambdaName string `yaml:"lambda_name"`
}

func (l Lambda) Pattern() string {
	lastDot := strings.LastIndex(l.Type, ".")
	if lastDot == -1 {
		return ""
	}

	return l.Type[:lastDot]
}

func (l Lambda) TypeName() string {
	lastDot := strings.LastIndex(l.Type, ".")
	if lastDot == -1 {
		return ""
	}

	return l.Type[lastDot+1:]
}
