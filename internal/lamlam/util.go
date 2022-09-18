package lamlam

import (
	"bufio"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"os"
	"strings"
	"sync"
)

const (
	buildTag = "lamlam"
)

func convertUpperCamelCasePkgPath(pkgPath string) string {
	pkgPath = strings.Trim(pkgPath, "/")

	pkgPath = cases.Title(language.English).String(pkgPath)
	return strings.ReplaceAll(pkgPath, "/", "")
}

var (
	_moduleNameOnce sync.Once
	_moduleName     string
	_moduleNameErr  error
)

func getCurrentModuleName() (string, error) {
	_moduleNameOnce.Do(func() {
		file, err := os.Open("go.mod")
		if err != nil {
			_moduleNameErr = err
			return
		}
		defer file.Close()
		line, _, err := bufio.NewReader(file).ReadLine()
		if err != nil {
			_moduleNameErr = err
			return
		}

		_moduleName = strings.TrimPrefix(string(line), "module ")
	})
	return _moduleName, _moduleNameErr
}
