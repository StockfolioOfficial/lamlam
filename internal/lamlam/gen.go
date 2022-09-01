package lamlam

import (
	"bytes"
	"fmt"
	"github.com/stockfolioofficial/lamlam/internal/config"
	"go/ast"
	"go/format"
	"go/types"
	"golang.org/x/tools/go/packages"
	"strings"
)

func makeGen(pkg *packages.Package) *gen {
	g := &gen{
		pkg: pkg,
	}

	for _, s := range pkg.Syntax {
		for _, decl := range s.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}

			for _, spec := range gd.Specs {
				switch spec := spec.(type) {
				case *ast.ImportSpec:
					g.importSpecs = append(g.importSpecs, spec)
				case *ast.TypeSpec:
					g.setInterface(spec)
				}
			}
		}
	}

	return g
}

type gen struct {
	pkg         *packages.Package
	importSpecs []*ast.ImportSpec
	intf        *genInterface
}

type genImport struct {
	name      string
	aliasName string
	path      string
	used      bool
	except    bool
}

func (gi *genImport) Name() string {
	if gi.aliasName != "" {
		return gi.aliasName
	}

	return gi.name
}

func (g *gen) getImportMap() map[string]*genImport {
	res := make(map[string]*genImport)
	for _, spec := range g.importSpecs {
		path := spec.Path.Value
		key := strings.Trim(path, "\"")
		imp := res[key]
		if imp != nil {
			continue
		}

		imp = &genImport{
			path: path,
		}

		impPkg := g.pkg.Imports[key]
		if impPkg == nil {
			continue
		}

		imp.name = impPkg.Name

		if spec.Name != nil {
			imp.aliasName = spec.Name.Name
		}

		res[key] = imp
	}

	res[g.pkg.PkgPath] = &genImport{
		name:      "",
		aliasName: "",
		path:      "",
		used:      true,
		except:    true,
	}

	return res
}

func (g *gen) setInterface(spec *ast.TypeSpec) {
	typ, ok := spec.Type.(*ast.InterfaceType)
	if !ok {
		return
	}

	g.intf = &genInterface{
		g:    g,
		spec: spec,
		typ:  typ,
	}

	g.intf.methods = make([]*genMethod, 0, len(typ.Methods.List))
	for _, method := range typ.Methods.List {
		gMethod := g.newGenMethod(method)
		if gMethod == nil {
			//TODO: error
			return
		}
		g.intf.methods = append(g.intf.methods, gMethod)
	}
}

type genInterface struct {
	g *gen

	spec *ast.TypeSpec
	typ  *ast.InterfaceType

	methods []*genMethod
}

func (gi *genInterface) Name() string {
	if gi.spec == nil || gi.spec.Name == nil {
		return ""
	}
	return gi.spec.Name.Name
}

func (gi *genInterface) FilePath() string {
	return gi.g.pkg.Fset.File(gi.typ.Pos()).Name()
}

func (g *gen) newGenMethod(method *ast.Field) *genMethod {
	methodType, ok := method.Type.(*ast.FuncType)

	var params []*genValueType
	if methodType.Params != nil {
		params = make([]*genValueType, 0, len(methodType.Params.List))
	}

	var results []*genValueType
	if methodType.Results != nil {
		results = make([]*genValueType, 0, len(methodType.Results.List))

	}

	if len(method.Names) == 0 || !ok || cap(params) > 2 || cap(results) > 2 {
		//TODO: error
		return nil
	}

	res := &genMethod{
		g:           g,
		methodField: method,
		funcType:    methodType,
	}

	if cap(params) > 0 {
		for _, param := range methodType.Params.List {
			vt := g.newGenValueType(param)
			if vt == nil {
				// TODO: error
				return nil
			}

			params = append(params, vt)
		}

		res.params = params
	}

	if cap(results) > 0 {
		for _, result := range methodType.Results.List {
			vt := g.newGenValueType(result)
			if vt == nil {
				// TODO: error
				return nil
			}

			results = append(results, vt)
		}

		res.results = results
	}

	return res
}

type genMethod struct {
	g *gen

	methodField *ast.Field
	funcType    *ast.FuncType

	params  []*genValueType
	results []*genValueType
}

func (gm *genMethod) Name() string {
	if len(gm.methodField.Names) > 0 {
		return gm.methodField.Names[0].Name
	}

	return ""
}

func (g *gen) newGenValueType(field *ast.Field) *genValueType {
	typ := g.pkg.TypesInfo.TypeOf(field.Type)
	if typ == nil {
		//TODO: error
		return nil
	}

	return &genValueType{
		g:     g,
		field: field,
		typ:   typ,
	}
}

type genValueType struct {
	g *gen

	field *ast.Field
	typ   types.Type
}

func (gvt *genValueType) Name() string {
	names := gvt.field.Names
	if len(names) > 0 {
		return names[0].Name
	}

	return ""
}

func newGenType(typ types.Type, importMap map[string]*genImport) *genType {
	switch typ := typ.(type) {
	case *types.Named:
		obj := typ.Obj()
		if obj == nil {
			return nil
		}

		res := &genType{
			typ: obj.Name(),
		}

		if pkg := obj.Pkg(); pkg != nil {
			imp := importMap[pkg.Path()]
			if imp == nil {
				return nil
			}

			imp.used = true
			res.pkg = imp.Name()
		}
		return res
	case *types.Slice:
		res := newGenType(typ.Elem(), importMap)
		if res == nil {
			return nil
		}

		res.prefixType = "[]" + res.prefixType
		return res
	case *types.Array:
		res := newGenType(typ.Elem(), importMap)
		if res == nil {
			return nil
		}

		res.prefixType = fmt.Sprintf("[%d]", typ.Len()) + res.prefixType
		return res
	case *types.Struct:
		var buf strings.Builder
		buf.WriteString("struct {\n")
		var gp *genType
		for i := 0; i < typ.NumFields(); i++ {
			f := typ.Field(i)
			gp = newGenType(f.Type(), importMap)
			if gp == nil {
				return nil
			}
			gp.fieldName = f.Id()
			buf.WriteString(gp.Name())
			buf.WriteRune('\n')
		}
		buf.WriteRune('}')

		return &genType{
			typ: buf.String(),
		}
	case *types.Basic:
		return &genType{
			typ: typ.Name(),
		}
	case *types.Pointer:
		res := newGenType(typ.Elem(), importMap)
		if res == nil {
			return nil
		}

		res.prefixType = "*" + res.prefixType
		return res
	default:
		return nil
	}
}

type genType struct {
	fieldName  string
	prefixType string
	typ        string
	pkg        string
}

func (gt *genType) Name() string {
	var buf strings.Builder
	if gt.fieldName != "" {
		buf.WriteString(gt.fieldName)
		buf.WriteRune(' ')
	}

	buf.WriteString(gt.prefixType)
	if gt.pkg != "" {
		buf.WriteString(gt.pkg)
		buf.WriteRune('.')
	}

	buf.WriteString(gt.typ)
	return buf.String()
}

func generate(g *gen, lambda *config.Lambda) ([]byte, error) {
	if g.intf == nil {
		return []byte{}, nil
	}

	typName := g.intf.Name()
	if typName == "" {
		//TODO: error
		return []byte{}, nil
	}

	importMap := g.getImportMap()
	funcName := formatConstLambdaName(typName)
	implTypName := fmt.Sprintf("%sImpl", strings.ToLower(typName))
	var body bytes.Buffer

	// ==== function keys ====
	body.WriteString("const (\n")
	body.WriteRune('\t')
	body.WriteString(funcName)
	body.WriteString(fmt.Sprintf(" = \"%s\"\n", lambda.LambdaName))
	for _, method := range g.intf.methods {
		methodName := method.Name()
		if methodName == "" {
			//TODO: error
			return []byte{}, nil
		}
		body.WriteRune('\t')
		body.WriteString(formatConstFuncKey(typName, methodName))
		body.WriteString(fmt.Sprintf(" = \"%s_%s\"\n", typName, methodName))
	}
	body.WriteString(")\n\n")

	// ===== handler =====
	body.WriteString("func New")
	body.WriteString(typName)
	body.WriteString("Handler(cli *lambda.Client) ")
	body.WriteString(typName)
	body.WriteString(" {\n")
	body.WriteString("\treturn &")
	body.WriteString(implTypName)
	body.WriteString("{\n")
	body.WriteString("\t\tinvoker: lamlam.NewInvoker(cli, ")
	body.WriteString(funcName)
	body.WriteString("),\n")
	body.WriteString("\t}\n")
	body.WriteString("}\n\n")

	body.WriteString("type ")
	body.WriteString(implTypName)
	body.WriteString(" struct {\n")
	body.WriteString("\tinvoker *lamlam.Invoker\n")
	body.WriteString("}\n\n")

	for _, method := range g.intf.methods {
		methodName := method.Name()
		if methodName == "" {
			//TODO: error
			return []byte{}, nil
		}

		body.WriteString("func (h *")
		body.WriteString(implTypName)
		body.WriteString(") ")
		body.WriteString(method.Name())

		body.WriteRune('(')

		hasContext, hasInput := false, false
		params := make([]string, 0, len(method.params))
		for _, param := range method.params {
			gTyp := newGenType(param.typ, importMap)
			if gTyp == nil {
				//TODO: error
				return []byte{}, nil
			}

			if gTyp.typ == "Context" {
				hasContext = true
				params = append(params, fmt.Sprintf("ctx %s", gTyp.Name()))
			} else {
				hasInput = true
				params = append(params, fmt.Sprintf("in %s", gTyp.Name()))
			}
		}

		body.WriteString(strings.Join(params, ", "))
		body.WriteRune(')')

		hasResult, hasError := false, false
		results := make([]string, 0, len(method.results))
		for _, result := range method.results {
			gTyp := newGenType(result.typ, importMap)
			if gTyp == nil {
				//TODO: error
				return []byte{}, nil
			}

			if gTyp.typ == "error" {
				hasError = true
				results = append(results, "err error")
			} else {
				hasResult = true
				results = append(results, fmt.Sprintf("res %s", gTyp.Name()))
			}
		}

		if len(results) > 0 {
			body.WriteString(" (")
			body.WriteString(strings.Join(results, ", "))
			body.WriteRune(')')
		}

		body.WriteString(" {\n")

		if hasError {
			body.WriteString("\terr = h.invoker.\n")
		} else {
			body.WriteString("\th.invoker.\n")
		}

		body.WriteString("\t\tFunc(")
		body.WriteString(formatConstFuncKey(typName, methodName))
		body.WriteString(").\n")

		body.WriteString("\t\tInvoke(")
		if hasContext {
			body.WriteString("ctx, ")
		} else {
			imp := importMap["context"]
			if imp == nil {
				imp = &genImport{
					name: "context",
					path: "\"context\"",
					used: true,
				}

				importMap["context"] = imp
			}
			body.WriteString("context.Background(), ")
		}

		if hasInput {
			body.WriteString("in).\n")
		} else {
			body.WriteString("nil).\n")
		}

		body.WriteString("\t\tResult(")
		if hasResult {
			body.WriteString("&res)\n")
		} else {
			body.WriteString("nil)\n")
		}
		body.WriteString("\treturn\n")
		body.WriteString("}\n\n")
	}

	// ==== mux ====
	body.WriteString("func NewMux")
	body.WriteString(typName)
	body.WriteString("(i ")
	body.WriteString(typName)
	body.WriteString(") *lamlam.Mux {\n")
	body.WriteString("\tm := lamlam.NewMux()\n")

	for _, method := range g.intf.methods {
		methodName := method.Name()
		if methodName == "" {
			//TODO: error
			return []byte{}, nil
		}

		body.WriteString("\tm.Set(")
		body.WriteString(formatConstFuncKey(typName, methodName))
		body.WriteString(", i.")
		body.WriteString(methodName)
		body.WriteString(")\n")
	}
	body.WriteString("\treturn m\n}\n\n")

	// ==== import ====
	var header bytes.Buffer
	writeGenHeader(&header)

	header.WriteString("package ")
	header.WriteString(g.pkg.Name)
	header.WriteString("\n\n")
	header.WriteString("import (\n")
	header.WriteString("\t\"github.com/stockfolioofficial/lamlam\"\n")
	header.WriteString("\t\"github.com/aws/aws-sdk-go-v2/service/lambda\"\n")
	for _, imp := range importMap {
		if !imp.used || imp.except {
			continue
		}

		header.WriteRune('\t')
		if imp.aliasName != "" {
			header.WriteString(imp.aliasName)
			header.WriteRune(' ')
		}
		header.WriteString(imp.path)
		header.WriteRune('\n')
	}
	header.WriteString(")\n\n")

	//return append(header.Bytes(), body.Bytes()...), nil
	return format.Source(append(header.Bytes(), body.Bytes()...))
}

//func generateFuncKey(g *gen, lambda *config.Lambda) ([]byte, error) {
//	if g.intf == nil {
//		return []byte{}, nil
//	}
//
//	typName := g.intf.Name()
//	if typName == "" {
//		//TODO: error
//		return []byte{}, nil
//	}
//
//	var buf bytes.Buffer
//	buf.WriteString("// Code generated by Wire. DO NOT EDIT.\n\n")
//	buf.WriteString("package ")
//	buf.WriteString(g.pkg.Name)
//	buf.WriteString("\n\n")
//
//	buf.WriteString("const (\n")
//	buf.WriteRune('\t')
//	buf.WriteString(formatConstLambdaName(typName))
//	buf.WriteString(fmt.Sprintf(" = \"%s\"\n", lambda.LambdaName))
//	for _, method := range g.intf.methods {
//		methodName := method.Name()
//		if methodName == "" {
//			//TODO: error
//			return []byte{}, nil
//		}
//		buf.WriteRune('\t')
//		buf.WriteString(formatConstFuncKey(typName, methodName))
//		buf.WriteString(fmt.Sprintf(" = \"%s_%s\"\n", typName, methodName))
//	}
//	buf.WriteString(")\n")
//
//	return format.Source(buf.Bytes())
//}

//func generateHandler(g *gen) ([]byte, error) {
//	if g.intf == nil {
//		return []byte{}, nil
//	}
//
//	typName := g.intf.Name()
//	if typName == "" {
//		//TODO: error
//		return []byte{}, nil
//	}
//
//	importMap := g.getImportMap()
//	funcName := formatConstLambdaName(typName)
//
//	implTypName := fmt.Sprintf("%sImpl", strings.ToLower(typName))
//	var body bytes.Buffer
//
//	body.WriteString("func New")
//	body.WriteString(typName)
//	body.WriteString("Handler(cli *lambda.Client) ")
//	body.WriteString(typName)
//	body.WriteString(" {\n")
//	body.WriteString("\treturn &")
//	body.WriteString(implTypName)
//	body.WriteString("{\n")
//	body.WriteString("\t\tinvoker: lamlam.NewInvoker(cli, ")
//	body.WriteString(funcName)
//	body.WriteString("),\n")
//	body.WriteString("\t}\n")
//	body.WriteString("}\n\n")
//
//	body.WriteString("type ")
//	body.WriteString(implTypName)
//	body.WriteString(" struct {\n")
//	body.WriteString("\tinvoker *lamlam.Invoker\n")
//	body.WriteString("}\n\n")
//
//	for _, method := range g.intf.methods {
//		methodName := method.Name()
//		if methodName == "" {
//			//TODO: error
//			return []byte{}, nil
//		}
//
//		body.WriteString("func (h *")
//		body.WriteString(implTypName)
//		body.WriteString(") ")
//		body.WriteString(method.Name())
//
//		body.WriteRune('(')
//
//		hasContext, hasInput := false, false
//		params := make([]string, 0, len(method.params))
//		for _, param := range method.params {
//			gTyp := newGenType(param.typ, importMap)
//			if gTyp == nil {
//				//TODO: error
//				return []byte{}, nil
//			}
//
//			if gTyp.typ == "Context" {
//				hasContext = true
//				params = append(params, fmt.Sprintf("ctx %s", gTyp.Name()))
//			} else {
//				hasInput = true
//				params = append(params, fmt.Sprintf("in %s", gTyp.Name()))
//			}
//		}
//
//		body.WriteString(strings.Join(params, ", "))
//		body.WriteRune(')')
//
//		hasResult, hasError := false, false
//		results := make([]string, 0, len(method.results))
//		for _, result := range method.results {
//			gTyp := newGenType(result.typ, importMap)
//			if gTyp == nil {
//				//TODO: error
//				return []byte{}, nil
//			}
//
//			if gTyp.typ == "error" {
//				hasError = true
//				results = append(results, "err error")
//			} else {
//				hasResult = true
//				results = append(results, fmt.Sprintf("res %s", gTyp.Name()))
//			}
//		}
//
//		if len(results) > 0 {
//			body.WriteString(" (")
//			body.WriteString(strings.Join(results, ", "))
//			body.WriteRune(')')
//		}
//
//		body.WriteString(" {\n")
//
//		if hasError {
//			body.WriteString("\terr = h.invoker.\n")
//		} else {
//			body.WriteString("\th.invoker.\n")
//		}
//
//		body.WriteString("\t\tFunc(")
//		body.WriteString(formatConstFuncKey(typName, methodName))
//		body.WriteString(").\n")
//
//		body.WriteString("\t\tInvoke(")
//		if hasContext {
//			body.WriteString("ctx, ")
//		} else {
//			imp := importMap["context"]
//			if imp == nil {
//				imp = &genImport{
//					name: "context",
//					path: "\"context\"",
//					used: true,
//				}
//
//				importMap["context"] = imp
//			}
//			body.WriteString("context.Background(), ")
//		}
//
//		if hasInput {
//			body.WriteString("in).\n")
//		} else {
//			body.WriteString("nil).\n")
//		}
//
//		body.WriteString("\t\tResult(")
//		if hasResult {
//			body.WriteString("&res)\n")
//		} else {
//			body.WriteString("nil)\n")
//		}
//		body.WriteString("\treturn\n")
//		body.WriteString("}\n\n")
//	}
//	var header bytes.Buffer
//	header.WriteString("// Code generated by Wire. DO NOT EDIT.\n\n")
//	header.WriteString("package ")
//	header.WriteString(g.pkg.Name)
//	header.WriteString("\n\n")
//	header.WriteString("import (\n")
//	for _, imp := range importMap {
//		if !imp.used || imp.except {
//			continue
//		}
//
//		header.WriteRune('\t')
//		if imp.aliasName != "" {
//			header.WriteString(imp.aliasName)
//			header.WriteRune(' ')
//		}
//		header.WriteString(imp.path)
//		header.WriteRune('\n')
//	}
//	header.WriteString(")\n\n")
//
//	return format.Source(append(header.Bytes(), body.Bytes()...))
//}

func writeGenHeader(header *bytes.Buffer) {
	header.WriteString("// Code generated by Wire. DO NOT EDIT.\n\n")

	header.WriteString("//go:build !")
	header.WriteString(buildTag)
	header.WriteRune('\n')

	header.WriteString("// +build !")
	header.WriteString(buildTag)
	header.WriteString("\n\n")
}
