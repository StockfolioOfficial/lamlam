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
				Type:       "github.com/stockfolioofficial/lamlam.SomeInterface1",
				LambdaName: "my-lambda-name1",
			},

			{
				Type:       "github.com/stockfolioofficial/lamlam.SomeInterface2",
				LambdaName: "my-lambda-name2",
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
