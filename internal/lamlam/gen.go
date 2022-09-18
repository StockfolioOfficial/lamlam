package lamlam

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/stockfolioofficial/lamlam/internal/config"
	"go/ast"
	"go/format"
	"go/types"
	"golang.org/x/tools/go/packages"
	"path/filepath"
	"strings"
)

type genFuncKeys struct {
	keys []genFuncKeyPair
}

type genFuncKeyPair struct {
	pkgPath       string
	interfaceName string
	methodName    string
}

type genMux struct {
	interfaces []genMuxInterface
}

type genMuxInterface struct {
	pkgPath string
	pkgName string
	typName string
	methods []string
}

type genHandler struct {
	implements []genImplement
}

type genImplement struct {
	pkgPath string
	pkgName string
	typName string
	methods []genImplementMethod
}

type genImplementMethod struct {
	methodName string
	params     []genImplementMethodValue
	results    []genImplementMethodValue
}

type genImplementMethodValue struct {
	pkgPaths []string
	typ      string
}

type importData struct {
	pkg       *packages.Package
	importPkg *packages.Package
	spec      *ast.ImportSpec
}

func (i importData) name() string {
	return i.importPkg.Name
}

type interfaceData struct {
	pkg     *packages.Package
	spec    *ast.TypeSpec
	typ     *ast.InterfaceType
	methods []methodData
}

func (i interfaceData) name() string {
	if i.spec == nil || i.spec.Name == nil {
		return ""
	}

	return i.spec.Name.Name
}

type methodData struct {
	pkg     *packages.Package
	field   *ast.Field
	typ     *ast.FuncType
	params  []*ast.Field
	results []*ast.Field
}

func (m methodData) name() string {
	if len(m.field.Names) > 0 {
		return m.field.Names[0].Name
	}

	return ""
}

func gen(pkgs []*packages.Package, lambda *config.Lambda) ([]byte, error) {
	moduleName, err := getCurrentModuleName()
	if err != nil {
		return nil, err
	}

	outputPkgPath := filepath.Join(moduleName, lambda.Output)
	targetTypeTable := make(map[string]*config.InterfaceType)
	for i := range lambda.Type {
		typ := &lambda.Type[i]
		pkgPath, _ := typ.GetPackagePath()
		targetTypeTable[pkgPath] = typ
	}

	var imports []importData
	var interfaces []interfaceData

	for _, pkg := range pkgs {
		if outputPkgPath == pkg.PkgPath {
			return nil, fmt.Errorf("cant output in \"%s\"", pkg.PkgPath)
		}

		imports = append(imports, importData{
			pkg:       pkg,
			importPkg: pkg,
			spec:      nil,
		})

		for _, s := range pkg.Syntax {
			for _, decl := range s.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok {
					continue
				}

				for _, spec := range gd.Specs {
					switch spec := spec.(type) {
					case *ast.ImportSpec:
						path := strings.Trim(spec.Path.Value, "\"")

						imp := pkg.Imports[path]
						if imp == nil {
							return nil, errors.New("not found import")
						}

						imports = append(imports, importData{
							pkg:       pkg,
							importPkg: imp,
							spec:      spec,
						})
					case *ast.TypeSpec:
						interfaceType, ok := spec.Type.(*ast.InterfaceType)
						if !ok {
							break
						}

						target := targetTypeTable[pkg.PkgPath]
						if target == nil {
							break
						}

						if typName, _ := target.GetTypeName(); spec.Name == nil || typName != spec.Name.Name {
							break
						}

						id := interfaceData{
							pkg:  pkg,
							spec: spec,
							typ:  interfaceType,
						}

						if interfaceType.Methods != nil {
							id.methods = make([]methodData, 0, len(interfaceType.Methods.List))
							for _, method := range interfaceType.Methods.List {

								methodType, ok := method.Type.(*ast.FuncType)
								if !ok {
									return nil, errors.New("must be *ast.FuncType")
								}

								md := methodData{
									pkg:    pkg,
									field:  method,
									typ:    methodType,
									params: methodType.Params.List,
								}

								if methodType.Results != nil {
									md.results = methodType.Results.List
								}

								id.methods = append(id.methods, md)
							}
						} // end - interfaceType.Methods != nil

						interfaces = append(interfaces, id)
					} // end - switch spec := spec.(type)
				} // end - for _, spec := range gd.Specs
			} // end - for _, decl := range s.Decls
		} // end - for _, s := range pkg.Syntax
	} // end - for _, pkg := range pkgs

	funcKey := makeGenFuncKeys(interfaces)
	mux := makeGenMux(interfaces)
	handler := makeGenHandler(interfaces)

	importTable := make(map[string]*importData)
	for i := range imports {
		imp := &imports[i]
		importTable[imp.importPkg.PkgPath] = imp
	}

	usedImports := make(map[string]*importData)
	for i := range handler.implements {
		impl := &handler.implements[i]
		if imp := importTable[impl.pkgPath]; imp != nil {
			usedImports[impl.pkgPath] = imp
		}

		for j := range impl.methods {
			method := &impl.methods[j]
			for x := range method.params {
				param := &method.params[x]

				for _, path := range param.pkgPaths {
					if imp := importTable[path]; imp != nil {
						usedImports[path] = imp
					}
				}
			}

			for x := range method.results {
				result := &method.results[x]

				for _, path := range result.pkgPaths {
					if imp := importTable[path]; imp != nil {
						usedImports[path] = imp
					}
				}
			}
		}
	}

	for i := range mux.interfaces {
		intface := &mux.interfaces[i]
		if imp := importTable[intface.pkgPath]; imp != nil {
			usedImports[intface.pkgPath] = imp
		}
	}

	var b bytes.Buffer
	writeGenHeader(&b)

	b.WriteString("package ")
	b.WriteString(filepath.Base(lambda.Output))
	b.WriteString("\n\n")

	packageNameTable := make(map[string]string)
	existsPackageNameTable := make(map[string]int)
	b.WriteString("import (\n")
	for key, val := range usedImports {
		pkgName := val.name()
		b.WriteRune('\t')
		if numbering := existsPackageNameTable[pkgName]; numbering > 0 {
			numberingPkgName := fmt.Sprintf("%s%d", pkgName, numbering)
			packageNameTable[key] = numberingPkgName
			b.WriteString(numberingPkgName)
			b.WriteRune(' ')
		} else {
			packageNameTable[key] = pkgName
		}
		b.WriteRune('"')
		b.WriteString(key)
		b.WriteString("\"\n")
		existsPackageNameTable[pkgName]++
	}
	b.WriteString("\t\"github.com/aws/aws-sdk-go-v2/service/lambda\"\n")
	b.WriteString("\t\"github.com/stockfolioofficial/lamlam\"\n")
	b.WriteString(")\n\n")

	b.WriteString("const (\n")
	b.WriteString("\tLambdaName = \"")
	b.WriteString(lambda.LambdaName)
	b.WriteString("\"\n")

	funcKeyNameTable := make(map[string]string)
	for i := range funcKey.keys {
		key := &funcKey.keys[i]
		pkgPath := convertUpperCamelCasePkgPath(strings.TrimPrefix(key.pkgPath, moduleName))

		name := fmt.Sprintf("FuncKey%s%s%s", pkgPath, key.interfaceName, key.methodName)
		value := fmt.Sprintf("%s.%s.%s", pkgPath, key.interfaceName, key.methodName)

		funcKeyNameTable[pkgPath+key.interfaceName+key.methodName] = name

		b.WriteRune('\t')
		b.WriteString(name)
		b.WriteString(" = \"")
		b.WriteString(value)
		b.WriteString("\"\n")
	}
	b.WriteString(")\n\n")

	for i := range handler.implements {
		impl := &handler.implements[i]
		pkgPath := convertUpperCamelCasePkgPath(strings.TrimPrefix(impl.pkgPath, moduleName))

		genTypeName := fmt.Sprintf("handler%s%sImpl", pkgPath, impl.typName)
		b.WriteString(fmt.Sprintf("func New%s%sHandler(cli *lambda.Client) %s.%s {\n", pkgPath, impl.typName, packageNameTable[impl.pkgPath], impl.typName))
		b.WriteString("\treturn &")
		b.WriteString(genTypeName)
		b.WriteString("{\n")
		b.WriteString("\t\tinvoker: lamlam.NewInvoker(cli, LambdaName),\n")
		b.WriteString("\t}\n")
		b.WriteString("}\n\n")

		b.WriteString("type ")
		b.WriteString(genTypeName)
		b.WriteString(" struct {\n")
		b.WriteString("\tinvoker *lamlam.Invoker\n")
		b.WriteString("}\n\n")

		for j := range impl.methods {
			method := &impl.methods[j]
			b.WriteString("func (h *")
			b.WriteString(genTypeName)
			b.WriteString(") ")
			b.WriteString(method.methodName)
			b.WriteRune('(')

			hasContext, hasInput := false, false
			params := make([]string, 0, 2)
			for x := range method.params {
				param := &method.params[x]

				if param.typ == "context.Context" {
					hasContext = true
					params = append(params, "ctx context.Context")
				} else {
					hasInput = true
					for _, path := range param.pkgPaths {
						imp := usedImports[path]
						if imp == nil {
							continue
						}
						param.typ = strings.ReplaceAll(param.typ, imp.name(), packageNameTable[path])
					}
					params = append(params, fmt.Sprintf("in %s", param.typ))
				}

			}
			b.WriteString(strings.Join(params, ", "))
			b.WriteString(")")

			hasResult, hasError := false, false
			results := make([]string, 0, 2)
			for x := range method.results {
				result := &method.results[x]

				if result.typ == "error" {
					hasError = true
					results = append(results, "err error")
				} else {
					hasResult = true
					for _, path := range result.pkgPaths {
						imp := usedImports[path]
						if imp == nil {
							continue
						}
						result.typ = strings.ReplaceAll(result.typ, imp.name(), packageNameTable[path])
					}
					results = append(results, fmt.Sprintf("res %s", result.typ))
				}
			}

			if len(results) > 0 {
				b.WriteString(" (")
				b.WriteString(strings.Join(results, ", "))
				b.WriteRune(')')
			}

			b.WriteString(" {\n\t")
			if hasError {
				b.WriteString("err = ")
			}
			b.WriteString("h.invoker.\n")
			b.WriteString("\tFunc(")
			b.WriteString(funcKeyNameTable[pkgPath+impl.typName+method.methodName])
			b.WriteString(").\n")
			b.WriteString("\tInvoke(")

			contextValue := "ctx"
			if !hasContext {
				contextValue = "context.Background()"
			}

			inputValue := "in"
			if !hasInput {
				inputValue = "nil"
			}

			b.WriteString(fmt.Sprintf("%s, %s).\n", contextValue, inputValue))
			b.WriteString("\tResult(")

			if hasResult {
				b.WriteString("&res)\n")
			} else {
				b.WriteString("nil)\n")
			}

			b.WriteString("\treturn\n")
			b.WriteString("}\n\n")
		}
	}

	for i := range mux.interfaces {
		intface := &mux.interfaces[i]
		pkgPath := convertUpperCamelCasePkgPath(strings.TrimPrefix(intface.pkgPath, moduleName))

		b.WriteString(fmt.Sprintf("func BindMux%s%s(m *lamlam.Mux, in %s.%s) {\n", pkgPath, intface.typName, packageNameTable[intface.pkgPath], intface.typName))
		for _, method := range intface.methods {
			b.WriteString("\tm.Set(")
			b.WriteString(funcKeyNameTable[pkgPath+intface.typName+method])
			b.WriteString(", in.")
			b.WriteString(method)
			b.WriteString(")\n")
		}

		b.WriteString("}\n\n")
	}

	return format.Source(b.Bytes())
}

func makeGenFuncKeys(interfaces []interfaceData) *genFuncKeys {
	var keys []genFuncKeyPair
	for i := range interfaces {
		intface := &interfaces[i]

		for j := range intface.methods {
			method := &intface.methods[j]

			keys = append(keys, genFuncKeyPair{
				pkgPath:       intface.pkg.PkgPath,
				interfaceName: intface.name(),
				methodName:    method.name(),
			})

		}
	}

	return &genFuncKeys{keys: keys}
}

func makeGenMux(interfaces []interfaceData) *genMux {
	genInterfaces := make([]genMuxInterface, 0, len(interfaces))
	for i := range interfaces {
		intface := &interfaces[i]

		gi := genMuxInterface{
			pkgPath: intface.pkg.PkgPath,
			pkgName: intface.pkg.Name,
			typName: intface.name(),
			methods: make([]string, 0, len(intface.methods)),
		}

		for j := range intface.methods {
			method := &intface.methods[j]
			gi.methods = append(gi.methods, method.name())
		}

		genInterfaces = append(genInterfaces, gi)
	}

	return &genMux{interfaces: genInterfaces}
}

func makeGenHandler(interfaces []interfaceData) *genHandler {
	implements := make([]genImplement, 0, len(interfaces))
	for i := range interfaces {
		intface := &interfaces[i]
		pkg := intface.pkg

		methods := make([]genImplementMethod, 0, len(intface.methods))
		for j := range intface.methods {
			method := &intface.methods[j]
			params := make([]genImplementMethodValue, 0, len(method.params))
			for _, param := range method.params {
				pt := pkg.TypesInfo.TypeOf(param.Type)
				params = append(params, genImplementMethodValue{
					pkgPaths: getTypePkgPaths(pt),
					typ:      getTypeName(pt),
				})
			}

			results := make([]genImplementMethodValue, 0, len(method.results))
			for _, result := range method.results {
				rt := pkg.TypesInfo.TypeOf(result.Type)
				results = append(results, genImplementMethodValue{
					pkgPaths: getTypePkgPaths(rt),
					typ:      getTypeName(rt),
				})
			}

			methods = append(methods, genImplementMethod{
				methodName: method.name(),
				params:     params,
				results:    results,
			})
		}

		implements = append(implements, genImplement{
			pkgPath: intface.pkg.PkgPath,
			pkgName: intface.pkg.Name,
			typName: intface.name(),
			methods: methods,
		})
	}

	return &genHandler{implements: implements}
}

func getTypePkgPaths(typ types.Type) []string {
	switch typ := typ.(type) {
	case *types.Named:
		obj := typ.Obj()
		if obj == nil {
			break
		}

		pkg := obj.Pkg()
		if pkg != nil {
			return []string{pkg.Path()}
		}
	case *types.Slice:
		return getTypePkgPaths(typ.Elem())
	case *types.Array:
		return getTypePkgPaths(typ.Elem())
	case *types.Struct:
		var res []string
		for i := 0; i < typ.NumFields(); i++ {
			f := typ.Field(i)
			paths := getTypePkgPaths(f.Type())
			if len(paths) > 0 {
				res = append(res, paths...)
			}
		}
		return res
	case *types.Pointer:
		return getTypePkgPaths(typ.Elem())
	}

	return nil
}

func getTypeName(typ types.Type) string {
	switch typ := typ.(type) {
	case *types.Named:
		obj := typ.Obj()
		if obj == nil {
			return "!unknown"
		}

		var b strings.Builder

		pkg := obj.Pkg()
		if pkg != nil {
			b.WriteString(pkg.Name())
			b.WriteRune('.')
		}
		b.WriteString(obj.Name())
		return b.String()
	case *types.Slice:
		var b strings.Builder
		b.WriteString("[]")
		b.WriteString(getTypeName(typ.Elem()))
		return b.String()
	case *types.Array:
		var b strings.Builder
		b.WriteString(fmt.Sprintf("[%d]", typ.Len()))
		b.WriteString(getTypeName(typ.Elem()))
		return b.String()
	case *types.Struct:
		var b strings.Builder
		b.WriteString("struct {\n")
		for i := 0; i < typ.NumFields(); i++ {
			f := typ.Field(i)
			b.WriteString(f.Name())
			b.WriteRune(' ')
			b.WriteString(getTypeName(f.Type()))
			b.WriteRune('\n')
		}
		b.WriteRune('}')
		return b.String()
	case *types.Basic:
		return typ.Name()
	case *types.Pointer:
		var b strings.Builder
		b.WriteString("*")
		b.WriteString(getTypeName(typ.Elem()))
		return b.String()
	default:
		return "!unknown"
	}
}

//
//func makeGen(pkg *packages.Package) *gen {
//	g := &gen{
//		pkg: pkg,
//	}
//
//	for _, s := range pkg.Syntax {
//		for _, decl := range s.Decls {
//			gd, ok := decl.(*ast.GenDecl)
//			if !ok {
//				continue
//			}
//
//			for _, spec := range gd.Specs {
//				switch spec := spec.(type) {
//				case *ast.ImportSpec:
//					g.importSpecs = append(g.importSpecs, spec)
//				case *ast.TypeSpec:
//					g.setInterface(spec)
//				}
//			}
//		}
//	}
//
//	return g
//}
//
//type gen struct {
//	pkg         *packages.Package
//	importSpecs []*ast.ImportSpec
//	intf        *genInterface
//}
//
//type genImport struct {
//	name      string
//	aliasName string
//	path      string
//	used      bool
//	except    bool
//}
//
//func (gi *genImport) Name() string {
//	if gi.aliasName != "" {
//		return gi.aliasName
//	}
//
//	return gi.name
//}
//
//func (g *gen) getImportMap() map[string]*genImport {
//	res := make(map[string]*genImport)
//	for _, spec := range g.importSpecs {
//		path := spec.Path.Value
//		key := strings.Trim(path, "\"")
//		imp := res[key]
//		if imp != nil {
//			continue
//		}
//
//		imp = &genImport{
//			path: path,
//		}
//
//		impPkg := g.pkg.Imports[key]
//		if impPkg == nil {
//			continue
//		}
//
//		imp.name = impPkg.Name
//
//		if spec.Name != nil {
//			imp.aliasName = spec.Name.Name
//		}
//
//		res[key] = imp
//	}
//
//	res[g.pkg.PkgPath] = &genImport{
//		name:      "",
//		aliasName: "",
//		path:      "",
//		used:      true,
//		except:    true,
//	}
//
//	return res
//}
//
//func (g *gen) setInterface(spec *ast.TypeSpec) {
//	typ, ok := spec.Type.(*ast.InterfaceType)
//	if !ok {
//		return
//	}
//
//	g.intf = &genInterface{
//		g:    g,
//		spec: spec,
//		typ:  typ,
//	}
//
//	g.intf.methods = make([]*genMethod, 0, len(typ.Methods.List))
//	for _, method := range typ.Methods.List {
//		gMethod := g.newGenMethod(method)
//		if gMethod == nil {
//			//TODO: error
//			return
//		}
//		g.intf.methods = append(g.intf.methods, gMethod)
//	}
//}
//
//type genInterface struct {
//	g *gen
//
//	spec *ast.TypeSpec
//	typ  *ast.InterfaceType
//
//	methods []*genMethod
//}
//
//func (gi *genInterface) Name() string {
//	if gi.spec == nil || gi.spec.Name == nil {
//		return ""
//	}
//	return gi.spec.Name.Name
//}
//
//func (gi *genInterface) FilePath() string {
//	return gi.g.pkg.Fset.File(gi.typ.Pos()).Name()
//}
//
//func (g *gen) newGenMethod(method *ast.Field) *genMethod {
//	methodType, ok := method.Type.(*ast.FuncType)
//
//	var params []*genValueType
//	if methodType.Params != nil {
//		params = make([]*genValueType, 0, len(methodType.Params.List))
//	}
//
//	var results []*genValueType
//	if methodType.Results != nil {
//		results = make([]*genValueType, 0, len(methodType.Results.List))
//
//	}
//
//	if len(method.Names) == 0 || !ok || cap(params) > 2 || cap(results) > 2 {
//		//TODO: error
//		return nil
//	}
//
//	res := &genMethod{
//		g:           g,
//		methodField: method,
//		funcType:    methodType,
//	}
//
//	if cap(params) > 0 {
//		for _, param := range methodType.Params.List {
//			vt := g.newGenValueType(param)
//			if vt == nil {
//				// TODO: error
//				return nil
//			}
//
//			params = append(params, vt)
//		}
//
//		res.params = params
//	}
//
//	if cap(results) > 0 {
//		for _, result := range methodType.Results.List {
//			vt := g.newGenValueType(result)
//			if vt == nil {
//				// TODO: error
//				return nil
//			}
//
//			results = append(results, vt)
//		}
//
//		res.results = results
//	}
//
//	return res
//}
//
//type genMethod struct {
//	g *gen
//
//	methodField *ast.Field
//	funcType    *ast.FuncType
//
//	params  []*genValueType
//	results []*genValueType
//}
//
//func (gm *genMethod) Name() string {
//	if len(gm.methodField.Names) > 0 {
//		return gm.methodField.Names[0].Name
//	}
//
//	return ""
//}
//
//func (g *gen) newGenValueType(field *ast.Field) *genValueType {
//	typ := g.pkg.TypesInfo.TypeOf(field.Type)
//	if typ == nil {
//		//TODO: error
//		return nil
//	}
//
//	return &genValueType{
//		g:     g,
//		field: field,
//		typ:   typ,
//	}
//}
//
//type genValueType struct {
//	g *gen
//
//	field *ast.Field
//	typ   types.Type
//}
//
//func (gvt *genValueType) Name() string {
//	names := gvt.field.Names
//	if len(names) > 0 {
//		return names[0].Name
//	}
//
//	return ""
//}
//
//func newGenType(typ types.Type, importMap map[string]*genImport) *genType {
//	switch typ := typ.(type) {
//	case *types.Named:
//		obj := typ.Obj()
//		if obj == nil {
//			return nil
//		}
//
//		res := &genType{
//			typ: obj.Name(),
//		}
//
//		if pkg := obj.Pkg(); pkg != nil {
//			imp := importMap[pkg.Path()]
//			if imp == nil {
//				return nil
//			}
//
//			imp.used = true
//			res.pkg = imp.Name()
//		}
//		return res
//	case *types.Slice:
//		res := newGenType(typ.Elem(), importMap)
//		if res == nil {
//			return nil
//		}
//
//		res.prefixType = "[]" + res.prefixType
//		return res
//	case *types.Array:
//		res := newGenType(typ.Elem(), importMap)
//		if res == nil {
//			return nil
//		}
//
//		res.prefixType = fmt.Sprintf("[%d]", typ.Len()) + res.prefixType
//		return res
//	case *types.Struct:
//		var buf strings.Builder
//		buf.WriteString("struct {\n")
//		var gp *genType
//		for i := 0; i < typ.NumFields(); i++ {
//			f := typ.Field(i)
//			gp = newGenType(f.Type(), importMap)
//			if gp == nil {
//				return nil
//			}
//			gp.fieldName = f.Id()
//			buf.WriteString(gp.Name())
//			buf.WriteRune('\n')
//		}
//		buf.WriteRune('}')
//
//		return &genType{
//			typ: buf.String(),
//		}
//	case *types.Basic:
//		return &genType{
//			typ: typ.Name(),
//		}
//	case *types.Pointer:
//		res := newGenType(typ.Elem(), importMap)
//		if res == nil {
//			return nil
//		}
//
//		res.prefixType = "*" + res.prefixType
//		return res
//	default:
//		return nil
//	}
//}
//
//type genType struct {
//	fieldName  string
//	prefixType string
//	typ        string
//	pkg        string
//}
//
//func (gt *genType) Name() string {
//	var buf strings.Builder
//	if gt.fieldName != "" {
//		buf.WriteString(gt.fieldName)
//		buf.WriteRune(' ')
//	}
//
//	buf.WriteString(gt.prefixType)
//	if gt.pkg != "" {
//		buf.WriteString(gt.pkg)
//		buf.WriteRune('.')
//	}
//
//	buf.WriteString(gt.typ)
//	return buf.String()
//}
//
//func generate(g *gen, lambda *config.Lambda) ([]byte, error) {
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
//	implTypName := fmt.Sprintf("%sImpl", strings.ToLower(typName))
//	var body bytes.Buffer
//
//	// ==== function keys ====
//	body.WriteString("const (\n")
//	body.WriteRune('\t')
//	body.WriteString(funcName)
//	body.WriteString(fmt.Sprintf(" = \"%s\"\n", lambda.LambdaName))
//	for _, method := range g.intf.methods {
//		methodName := method.Name()
//		if methodName == "" {
//			//TODO: error
//			return []byte{}, nil
//		}
//		body.WriteRune('\t')
//		body.WriteString(formatConstFuncKey(typName, methodName))
//		body.WriteString(fmt.Sprintf(" = \"%s_%s\"\n", typName, methodName))
//	}
//	body.WriteString(")\n\n")
//
//	// ===== handler =====
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
//
//	// ==== mux ====
//	body.WriteString("func NewMux")
//	body.WriteString(typName)
//	body.WriteString("(i ")
//	body.WriteString(typName)
//	body.WriteString(") *lamlam.Mux {\n")
//	body.WriteString("\tm := lamlam.NewMux()\n")
//
//	for _, method := range g.intf.methods {
//		methodName := method.Name()
//		if methodName == "" {
//			//TODO: error
//			return []byte{}, nil
//		}
//
//		body.WriteString("\tm.Set(")
//		body.WriteString(formatConstFuncKey(typName, methodName))
//		body.WriteString(", i.")
//		body.WriteString(methodName)
//		body.WriteString(")\n")
//	}
//	body.WriteString("\treturn m\n}\n\n")
//
//	// ==== import ====
//	var header bytes.Buffer
//	writeGenHeader(&header)
//
//	header.WriteString("package ")
//	header.WriteString(g.pkg.Name)
//	header.WriteString("\n\n")
//	header.WriteString("import (\n")
//	header.WriteString("\t\"github.com/stockfolioofficial/lamlam\"\n")
//	header.WriteString("\t\"github.com/aws/aws-sdk-go-v2/service/lambda\"\n")
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
//	//return append(header.Bytes(), body.Bytes()...), nil
//	return format.Source(append(header.Bytes(), body.Bytes()...))
//}

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
//	buf.WriteString("// Code generated by lamlam. DO NOT EDIT.\n\n")
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
//	header.WriteString("// Code generated by lamlam. DO NOT EDIT.\n\n")
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

func writeGenHeader(b *bytes.Buffer) {
	b.WriteString("// Code generated by lamlam. DO NOT EDIT.\n\n")

	b.WriteString("//go:build !")
	b.WriteString(buildTag)
	b.WriteRune('\n')

	b.WriteString("// +build !")
	b.WriteString(buildTag)
	b.WriteString("\n\n")
}
