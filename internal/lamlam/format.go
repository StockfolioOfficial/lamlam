package lamlam

import "fmt"

func formatConstFuncKey(typName string, methodName string) string {
	return fmt.Sprintf("FuncKey%s%s", typName, methodName)
}

func formatConstLambdaName(typName string) string {
	return fmt.Sprintf("LambdaName%s", typName)
}
