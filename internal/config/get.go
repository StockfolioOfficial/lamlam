package config

import (
	"gopkg.in/yaml.v3"
	"os"
)

func GetInitConfigVersion1() *Config {
	return &Config{
		Version: Version1,
		LambdaList: []Lambda{
			{
				Type: InterfaceTypes{
					"github.com/stockfolioofficial/lamlam.SomeInterface1",
					"github.com/stockfolioofficial/lamlam.SomeInterface2",
				},
				LambdaName: "my-lambda-name1",
				Output:     "./infra/foo",
			},

			{
				Type: InterfaceTypes{
					"github.com/stockfolioofficial/lamlam.SomeInterface3",
				},
				LambdaName: "my-lambda-name2",
				Output:     "infra/bar",
			},
		},
	}
}

func GetFromFile(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	defer file.Close()
	dec := yaml.NewDecoder(file)

	var cfg Config
	err = dec.Decode(&cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
