package config

import (
	"errors"
	"gopkg.in/yaml.v3"
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

type Lambda struct {
	Type       InterfaceTypes `yaml:"type"`
	LambdaName string         `yaml:"lambda_name"`
	Output     string         `yaml:"output"`
}

type InterfaceTypes []InterfaceType

func (list InterfaceTypes) MarshalYAML() (interface{}, error) {
	switch len(list) {
	case 0:
		return nil, nil
	case 1:
		return list[0], nil
	}

	return []InterfaceType(list), nil
}

func (list *InterfaceTypes) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.SequenceNode:
		for _, node := range value.Content {
			if node.Kind != yaml.ScalarNode {
				return errors.New("unsupported node type, must be scalar type")
			}

			*list = append(*list, InterfaceType(node.Value))
		}
	case yaml.ScalarNode:
		*list = append(*list, InterfaceType(value.Value))
	default:
		return errors.New("unsupported node type")
	}
	return nil
}

type InterfaceType string

func (i InterfaceType) Divide() (pkg, typeName string, err error) {
	pkgTypeName := string(i)
	lastDot := strings.LastIndex(pkgTypeName, ".")
	if lastDot == -1 {
		err = errors.New("must include Type name")
		return
	}

	pkg, typeName = pkgTypeName[:lastDot], pkgTypeName[lastDot+1:]
	return
}

func (i InterfaceType) GetPackagePath() (pkg string, err error) {
	pkg, _, err = i.Divide()
	return
}

func (i InterfaceType) GetTypeName() (typeName string, err error) {
	_, typeName, err = i.Divide()
	return
}
