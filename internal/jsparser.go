package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

type Schema struct {
	Schema            *string               `json:"$schema"`
	ID                *string               `json:"$id"`
	Title             *string               `json:"title"`
	Description       *string               `json:"description"`
	Type              Types                 `json:"type"`
	Definitions       map[string]Properties `json:"definitions"`
	PatternProperties PatternProperties     `json:"patternProperties"`
	Required          []string              `json:"required"`
	Defs              Defs                  `json:"$defs"`
}

type Defs struct {
	DiskDevice Properties `json:"diskDevice"`
	DiskUUID   Properties `json:"diskUUID"`
	NFS        Properties `json:"nfs"`
	Tmpfs      Properties `json:"tmpfs"`
}

type PatternProperties struct {
	Empty Empty `json:"^(/[^/]+)+$"`
}

type Empty struct {
	Ref *string `json:"$ref"`
}

type Properties struct {
	Description       *string               `json:"description"`
	Type              Types                 `json:"type"`
	ExclusiveMinimum  *int64                `json:"exclusiveMinimum"`
	Items             *Properties           `json:"items"`
	MinItems          *int64                `json:"minItems"`
	UniqueItems       *bool                 `json:"uniqueItems"`
	Ref               *string               `json:"$ref"`
	OneOf             []*Properties         `json:"oneOf"`
	AllOf             []*Properties         `json:"allOf"`
	AnyOf             []*Properties         `json:"anyOf"`
	Enum              []string              `json:"enum"`
	Pattern           *string               `json:"pattern"`
	Minimum           *int64                `json:"minimum"`
	Maximum           *int64                `json:"maximum"`
	Format            string                `json:"format"`
	Properties        map[string]Properties `json:"properties"`
	Required          []string              `json:"required"`
	PatternProperties PatternProperties     `json:"patternProperties"`
	Defs              *Defs                 `json:"$defs"`
}

type Items struct {
	Type *string `json:"type"`
}

type NestedObjectHandler func(name string, value any)
type DuplicateCheck func(typeName string) bool

type Types string

const (
	NONE    Types = ""
	STRING  Types = "string"
	NUMBER  Types = "number"
	INTEGER Types = "integer"
	OBJECT  Types = "object"
	ARRAY   Types = "array"
	BOOLEAN Types = "boolean"
	NULL    Types = "null"
	ANY     Types = "Any"
)

type PropertyType int

const (
	ENUM_TYPE PropertyType = iota
	UNION_TYPE
	COMPLEX_ARRAY_TYPE
	REF_ARRAY_TYPE
	PRIMITIVE_ARRAY_TYPE
	UNKOWN_ARRAY_TYPE
	NESTED_OBJECT_TYPE
	REF_TYPE
	PRIMITIVE_TYPE
)

func (properties Properties) GetType() PropertyType {
	if properties.Enum != nil {
		return ENUM_TYPE
	}
	if properties.AnyOf != nil {
		return UNION_TYPE
	}
	if properties.Type == ARRAY {
		if properties.Items.Type == NONE && properties.Items.Ref == nil {
			return UNKOWN_ARRAY_TYPE
		}
		if properties.Items.Ref != nil {
			return REF_ARRAY_TYPE
		}
		if properties.Items.Type == OBJECT {
			return COMPLEX_ARRAY_TYPE
		}
		return PRIMITIVE_ARRAY_TYPE
	}
	if properties.Type == OBJECT {
		return NESTED_OBJECT_TYPE
	}
	if properties.Ref != nil {
		return REF_TYPE
	}
	return PRIMITIVE_TYPE
}

func (properties Properties) GetRef(root map[string]Properties) (key string, value map[string]Properties) {
	if strings.HasPrefix(strings.ToLower(*properties.Ref), "http") {
		panic("External Json Schemas are not supported by J2P compiler")
	}
	path := strings.Split(*properties.Ref, "/")
	len := len(path)
	ref := root
	for i := 1; i < len; i++ {
		if i == 1 {
			if path[i] == "$defs" {
				panic("$defs is a Json Schema specification which is not supported by J2P compiler")
			}
			if path[i] == "definitions" {
				continue
			}
		}
		ref = ref[path[i]].Properties
	}
	return path[len-1], ref
}

func (properties Properties) GetRefType(root map[string]Properties) string {
	if properties.Ref == nil {
		return ""
	}
	path := strings.Split(*properties.Ref, "/")
	len := len(path)
	return path[len-1]
}

func (properties Properties) ToField(root map[string]Properties, propertyName string, index *int, nestedObjectHander NestedObjectHandler) string {
	_type := properties.GetType()
	switch _type {
	case PRIMITIVE_TYPE:
		{
			return ToPrimitiveProperty(propertyName, properties.Type, index)
		}
	case REF_TYPE:
		{
			refType, ref := properties.GetRef(root)
			nestedObjectHander(propertyName, ref)
			return ToRefProperty(propertyName, refType, index)
		}
	case PRIMITIVE_ARRAY_TYPE:
		{
			return ToPrimitiveArrayProperty(propertyName, properties.Items.Type, index)
		}
	case UNKOWN_ARRAY_TYPE:
		{
			return ToRefArrayProperty(propertyName, "google.protobuf.Any", index)
		}
	case REF_ARRAY_TYPE:
		{
			refType, ref := properties.Items.GetRef(root)
			nestedObjectHander(propertyName, ref)
			return ToRefArrayProperty(propertyName, refType, index)
		}
	case COMPLEX_ARRAY_TYPE:
		{
			break
		}
	case ENUM_TYPE:
		{
			nestedObjectHander(propertyName, properties.Enum)
			return ToRefProperty(propertyName, propertyName, index)
		}
	case NESTED_OBJECT_TYPE:
		{
			nestedObjectHander(propertyName, properties)
			return ToRefProperty(propertyName, propertyName, index)
		}
	case UNION_TYPE:
		{
			return ToUnionProperty(root, propertyName, properties.AnyOf, index, nestedObjectHander)
		}
	}
	return "--Invalid Type--"
}

const MESSAGE_TEMPLATE = `
message _$NAME$_ {
_$VALUE$_	
}
`

func ToMessage(root map[string]Properties, messageName string, properties map[string]Properties, nestedObjectHandler NestedObjectHandler, duplicateCheck DuplicateCheck) string {
	typeName := toPascalCase(messageName)
	if duplicateCheck(*typeName) {
		return ""
	}
	buffer := bytes.NewBufferString("")
	keys := make([]string, 0)
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) < len(keys[j])
	})
	index := 1
	for _, key := range keys {
		value := properties[key]
		buffer.WriteString(value.ToField(root, key, &index, nestedObjectHandler))
		buffer.WriteString("\n")
	}
	renderedStr := MESSAGE_TEMPLATE
	renderedStr = strings.Replace(renderedStr, "_$NAME$_", *typeName, 1)
	renderedStr = strings.Replace(renderedStr, "_$VALUE$_", buffer.String(), 1)
	return renderedStr
}

func (schema Schema) ToProtobuf(root map[string]Properties, nestedObjectHandler NestedObjectHandler, duplicateCheck DuplicateCheck) string {
	buffer := bytes.NewBufferString("")
	keys := make([]string, 0)
	for key := range schema.Definitions {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) < len(keys[j])
	})
	for _, key := range keys {
		value := schema.Definitions[key]
		_type := value.GetType()
		switch _type {
		case ENUM_TYPE:
			{
				buffer.WriteString(ToEnum(key, value.Enum, duplicateCheck))
			}
		default:
			{
				buffer.WriteString(ToMessage(root, key, value.Properties, nestedObjectHandler, duplicateCheck))
			}
		}
		buffer.WriteString("\n")
	}
	return buffer.String()
}

const ENUM_TEMPLATE = `
enum _$NAME$_ {
_$VALUE$_
}
`

func ToEnum(enumName string, enumValue []string, duplicateCheck DuplicateCheck) string {
	_enumName := toPascalCase(enumName)
	if duplicateCheck(*_enumName) {
		return ""
	}
	buffer := bytes.NewBufferString("")
	for index, value := range enumValue {
		fixedValue := *fixString(value)
		buffer.WriteString("\t")
		buffer.WriteString(strings.ToUpper(fmt.Sprintf("%s_%s", *_enumName, fixedValue)))
		buffer.WriteString(" ")
		buffer.WriteString("=")
		buffer.WriteString(" ")
		buffer.WriteString(fmt.Sprintf("%d", index))
		buffer.WriteString(";\n")
	}
	renderedStr := ENUM_TEMPLATE
	renderedStr = strings.Replace(renderedStr, "_$NAME$_", *_enumName, 1)
	renderedStr = strings.Replace(renderedStr, "_$VALUE$_", buffer.String(), 1)
	return renderedStr
}

const UNION_TEMPLATE = `
	oneof _$NAME$__union {
_$VALUE$_
	}
`

func ToUnionProperty(root map[string]Properties, unionName string, unionValue []*Properties, index *int, nestedObjectHandler NestedObjectHandler) string {
	buffer := bytes.NewBufferString("")
	if len(unionValue) == 2 {
		isOptional := false
		var _value *Properties
		for _, value := range unionValue {
			if value.Type == NULL {
				isOptional = true
			} else {
				_value = value
			}

		}
		if isOptional {
			_type := string(_value.Type)
			if _value.Type == NONE {
				_type = _value.GetRefType(root)
			}
			if len(_type) == 0 {
				panic("Unions without types or formatted unions are not supported by J2P")
			}
			return fmt.Sprintf("\toptional %s", strings.TrimLeft(_value.ToField(root, fmt.Sprintf("%s_%s", *toCamelCase(unionName), *toCamelCase(_type)), index, nestedObjectHandler), "\t"))
		}
	}
	for _, value := range unionValue {
		buffer.WriteString("\t")
		_type := string(value.Type)
		if value.Type == NONE {
			_type = value.GetRefType(root)
		}
		if len(_type) == 0 {
			panic("Unions without types or formatted unions are not supported by J2P")
		}
		buffer.WriteString(value.ToField(root, fmt.Sprintf("%s_%s", *toCamelCase(unionName), *toCamelCase(_type)), index, nestedObjectHandler))
		buffer.WriteString("\n")
	}
	renderedStr := UNION_TEMPLATE
	renderedStr = strings.Replace(renderedStr, "_$NAME$_", *toCamelCase(unionName), 1)
	renderedStr = strings.Replace(renderedStr, "_$VALUE$_", buffer.String(), 1)
	return renderedStr
}

func ToPrimitiveProperty(propertyName string, typeName Types, index *int) string {
	var _typename string
	switch typeName {
	case INTEGER:
		{
			_typename = "int32"
			break
		}
	case NUMBER:
		{
			_typename = "double"
			break
		}
	case BOOLEAN:
		{
			_typename = "bool"
			break
		}
	case NULL:
		{
			_typename = "optional google.protobuf.Any"
		}
	default:
		{
			if typeName == "String" {
				_typename = "string"
			} else {
				_typename = string(typeName)
			}
			break
		}
	}
	var output string
	snakeCasePropertyName, ok := toSnakeCase(propertyName)
	if ok {
		output = fmt.Sprintf("\t%s %s = %d [json_name=\"%s\"];", _typename, *toCamelCase(propertyName), *index, *snakeCasePropertyName)
	} else {
		output = fmt.Sprintf("\t%s %s = %d;", _typename, *toCamelCase(propertyName), *index)
	}
	*index += 1
	return output
}

func ToPrimitiveArrayProperty(propertyName string, typeName Types, index *int) string {
	var output string
	output = fmt.Sprintf("\trepeated %s", strings.TrimPrefix(ToPrimitiveProperty(propertyName, typeName, index), "\t"))
	return output
}

func ToRefArrayProperty(propertyName string, typeName string, index *int) string {
	var output string
	snakeCasePropertyName, ok := toSnakeCase(propertyName)
	if ok {
		output = fmt.Sprintf("\trepeated %s %s = %d [json_name=\"%s\"];", *toPascalCase(typeName), *toCamelCase(propertyName), *index, *snakeCasePropertyName)
	} else {
		output = fmt.Sprintf("\trepeated %s %s = %d;", *toPascalCase(typeName), *toCamelCase(propertyName), *index)
	}
	*index += 1
	return output
}

func ToRefProperty(propertyName string, typeName string, index *int) string {
	var output string
	snakeCasePropertyName, ok := toSnakeCase(propertyName)
	if ok {
		output = fmt.Sprintf("\t%s %s = %d [json_name=\"%s\"];", *toPascalCase(typeName), *toCamelCase(propertyName), *index, *snakeCasePropertyName)
	} else {
		output = fmt.Sprintf("\t%s %s = %d;", *toPascalCase(typeName), *toCamelCase(propertyName), *index)
	}
	*index += 1
	return output
}

type DefaultJsonSchemaParser struct {
	schema             Schema
	pushBacks          map[string]any
	nestedObjectHander NestedObjectHandler
	typeNames          []string
	duplicateCheck     DuplicateCheck
}

func New(jsonSchema []byte) DefaultJsonSchemaParser {
	schema := Schema{}
	err := json.Unmarshal(jsonSchema, &schema)
	if err != nil {
		panic(err)
	}
	output := DefaultJsonSchemaParser{}
	output.schema = schema
	output.pushBacks = make(map[string]any)
	output.typeNames = make([]string, 0)
	output.nestedObjectHander = func(name string, value any) {
		output.pushBacks[name] = value
	}
	output.duplicateCheck = func(typeName string) bool {
		for _, value := range output.typeNames {
			if value == typeName {
				return true
			}
		}
		output.typeNames = append(output.typeNames, typeName)
		return false
	}
	return output
}

const HEADERS = `
syntax = "proto3";

package _$PACKAGE$_;

import "google/protobuf/any.proto";

`

func (rcvr DefaultJsonSchemaParser) Parse(packageName string) []string {
	values := make([]string, 0)
	values = append(values, strings.Replace(HEADERS, "_$PACKAGE$_", packageName, 1))
	values = append(values, rcvr.schema.ToProtobuf(rcvr.schema.Definitions, rcvr.nestedObjectHander, rcvr.duplicateCheck))
	for len(rcvr.pushBacks) > 0 {
		keys := make([]string, 0)
		for key, value := range rcvr.pushBacks {
			keys = append(keys, key)
			if _value, ok := value.(map[string]Properties); ok {
				values = append(values, ToMessage(rcvr.schema.Definitions, key, _value, rcvr.nestedObjectHander, rcvr.duplicateCheck))
				continue
			}
			if _value, ok := value.(Properties); ok {
				values = append(values, ToMessage(rcvr.schema.Definitions, key, _value.Properties, rcvr.nestedObjectHander, rcvr.duplicateCheck))
				continue
			}
			if _value, ok := value.([]string); ok {
				values = append(values, ToEnum(key, _value, rcvr.duplicateCheck))
				continue
			}
		}
		for _, key := range keys {
			delete(rcvr.pushBacks, key)
		}
	}
	return values
}

func fixString(str string) *string {
	output := str
	output = strings.ReplaceAll(output, "#", "_")
	output = strings.ReplaceAll(output, " ", "_")
	output = strings.ReplaceAll(output, "-", "_")
	output = strings.ReplaceAll(output, "/", "_")
	output = strings.ReplaceAll(output, ".", "_")
	return &output
}

func toCamelCase(str string) *string {
	fixedStr := fixString(str)
	var output string
	if fixedStr == nil {
		output = ""
		return &output
	}
	_str := []byte(*fixedStr)
	if len(_str) == 0 {
		output = ""
		return &output
	}
	_str[0] = byte(strings.ToLower(string(_str[0]))[0])
	output = string(_str)
	return &output
}

func toPascalCase(str string) *string {
	if strings.Contains(str, ".") {
		return &str
	}
	fixedStr := fixString(str)
	var output string
	if fixedStr == nil {
		output = ""
		return &output
	}
	_str := []byte(*fixedStr)
	if len(_str) == 0 {
		output = ""
		return &output
	}
	_str[0] = byte(strings.ToUpper(string(_str[0]))[0])
	output = string(_str)
	return &output
}

func toSnakeCase(str string) (*string, bool) {
	fixedStr := fixString(str)
	var output bytes.Buffer
	if fixedStr == nil {
		outputStr := output.String()
		return &outputStr, false
	}
	_str := []byte(*fixedStr)
	if len(_str) == 0 {
		outputStr := output.String()
		return &outputStr, false
	}
	isConverted := false
	isPrevNum := false
	isPrevUnderline := false
	for index, value := range _str {
		if value == '_' {
			isConverted = true
			isPrevUnderline = true
			output.WriteString(strings.ToLower(string(value)))
			continue
		}
		if unicode.IsUpper(rune(value)) {
			if index != 0 {
				output.WriteString("_")
				isConverted = true
			}
			output.WriteString(strings.ToLower(string(value)))
			continue
		}
		if unicode.IsNumber(rune(value)) {
			if !isPrevNum {
				if index != 0 {
					output.WriteString("_")
					isConverted = true
				}
				isPrevNum = true
				output.WriteString(strings.ToLower(string(value)))
				continue
			}
			output.WriteString(strings.ToLower(string(value)))
			continue
		}
		if isPrevNum {
			output.WriteString("_")
			isConverted = true
			isPrevNum = false
			output.WriteString(strings.ToLower(string(value)))
			continue
		}
		if isPrevUnderline {
			isPrevUnderline = false
		}
		output.WriteByte(value)
	}
	outputStr := output.String()
	return &outputStr, isConverted
}
