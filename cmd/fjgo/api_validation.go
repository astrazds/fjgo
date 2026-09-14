package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/astrazds/fjgo/internal/forgejo"
)

func validateOperationQuery(op forgejo.Operation, query url.Values) error {
	for _, param := range op.QueryParams {
		values, present := query[param.Name]
		if param.Required && (!present || len(values) == 0 || values[0] == "") {
			return newUsageError(
				fmt.Sprintf("missing required query parameter %q for %s", param.Name, op.ID),
				fmt.Sprintf("Run `fjgo api inspect %s` to see query requirements", op.ID),
			)
		}
		for _, value := range values {
			if err := validateOperationParam(param, value); err != nil {
				return newUsageError(fmt.Sprintf("invalid %s query parameter %q: %v", op.ID, param.Name, err))
			}
		}
	}
	return nil
}

func operationQueryParam(op forgejo.Operation, name string) (forgejo.OperationParam, bool) {
	for _, param := range op.QueryParams {
		if param.Name == name {
			return param, true
		}
	}
	return forgejo.OperationParam{}, false
}

func operationPathParam(op forgejo.Operation, name string) (forgejo.OperationParam, bool) {
	for _, param := range op.PathParamInfo {
		if param.Name == name {
			return param, true
		}
	}
	return forgejo.OperationParam{}, false
}

func validateOperationPath(op forgejo.Operation, values map[string]string) error {
	for _, param := range op.PathParamInfo {
		value, present := values[param.Name]
		if param.Required && (!present || value == "") {
			return newUsageError(fmt.Sprintf("missing required path parameter %q for %s", param.Name, op.ID))
		}
		if present {
			if err := validateOperationParam(param, value); err != nil {
				return newUsageError(fmt.Sprintf("invalid %s path parameter %q: %v", op.ID, param.Name, err))
			}
		}
	}
	return nil
}

func validateOperationUpload(op forgejo.Operation, query url.Values, fields map[string]string, files []forgejo.UploadPart) error {
	if err := validateOperationQuery(op, query); err != nil {
		return err
	}
	fileFields := map[string]bool{}
	for _, file := range files {
		fileFields[file.FieldName] = true
	}
	for _, param := range op.FormParams {
		value, hasValue := fields[param.Name]
		hasFile := fileFields[param.Name]
		if param.Required && !hasValue && !hasFile {
			return newUsageError(fmt.Sprintf("missing required form parameter %q for %s", param.Name, op.ID))
		}
		if hasValue {
			if err := validateOperationParam(param, value); err != nil {
				return newUsageError(fmt.Sprintf("invalid %s form parameter %q: %v", op.ID, param.Name, err))
			}
		}
	}
	if op.ID == "repoCreateReleaseAttachment" && len(files) != 0 && fields["external_url"] != "" {
		return newUsageError("repoCreateReleaseAttachment accepts attachment or external_url, not both")
	}
	return nil
}

func validateOperationParam(param forgejo.OperationParam, value string) error {
	values := []string{value}
	if strings.HasPrefix(param.Type, "[]") && param.CollectionFormat != "multi" {
		separator := ","
		switch param.CollectionFormat {
		case "ssv":
			separator = " "
		case "tsv":
			separator = "\t"
		case "pipes":
			separator = "|"
		}
		values = strings.Split(value, separator)
	}
	baseType := strings.TrimPrefix(param.Type, "[]")
	for _, item := range values {
		if len(param.Enum) != 0 && !slices.Contains(param.Enum, item) {
			return fmt.Errorf("expected one of %s, got %q", strings.Join(param.Enum, ", "), item)
		}
		switch strings.SplitN(baseType, ":", 2)[0] {
		case "integer", "int", "int64":
			parsed, err := strconv.ParseInt(item, 10, 64)
			if err != nil {
				return fmt.Errorf("expected integer, got %q", item)
			}
			if param.HasMinimum && float64(parsed) < param.Minimum {
				return fmt.Errorf("expected at least %s, got %q", strconv.FormatFloat(param.Minimum, 'g', -1, 64), item)
			}
		case "number", "float64":
			parsed, err := strconv.ParseFloat(item, 64)
			if err != nil {
				return fmt.Errorf("expected number, got %q", item)
			}
			if param.HasMinimum && parsed < param.Minimum {
				return fmt.Errorf("expected at least %s, got %q", strconv.FormatFloat(param.Minimum, 'g', -1, 64), item)
			}
		case "boolean", "bool":
			if item != "true" && item != "false" {
				return fmt.Errorf("expected true or false, got %q", item)
			}
		}
	}
	return nil
}

func validateOperationBody(op forgejo.Operation, body any) error {
	if body == nil {
		return nil
	}
	if op.BodyType == "" {
		return newUsageError(fmt.Sprintf("%s does not declare a JSON request body", op.ID), "Use `fjgo api raw` for undocumented request shapes")
	}
	if err := validateSchemaValue(op.BodyType, body, "body", map[string]bool{}); err != nil {
		return newUsageError(fmt.Sprintf("invalid body for %s: %v", op.ID, err), fmt.Sprintf("Run `fjgo model inspect %s` for the body schema", strings.TrimPrefix(op.BodyType, "*")))
	}
	return nil
}

func normalizeOperationBody(op forgejo.Operation, input *apiBodyInput) error {
	if input == nil || input.JSON == nil || input.Raw != nil || op.BodyType != "string" || operationConsumesJSON(op) {
		return nil
	}
	text, ok := input.JSON.(string)
	if !ok {
		return newUsageError(fmt.Sprintf("%s requires a string body", op.ID))
	}
	input.JSON = nil
	input.Raw = []byte(text)
	if input.ContentType == "" && len(op.Consumes) != 0 {
		input.ContentType = op.Consumes[0]
	}
	return nil
}

func operationConsumesJSON(op forgejo.Operation) bool {
	for _, mediaType := range op.Consumes {
		if strings.Contains(strings.ToLower(mediaType), "json") {
			return true
		}
	}
	return false
}

func validateSchemaValue(typeName string, value any, path string, visiting map[string]bool) error {
	typeName = strings.TrimSpace(typeName)
	if strings.HasPrefix(typeName, "[]") {
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		itemType := strings.TrimPrefix(typeName, "[]")
		for i, item := range items {
			if err := validateSchemaValue(itemType, item, fmt.Sprintf("%s[%d]", path, i), visiting); err != nil {
				return err
			}
		}
		return nil
	}
	typeName = strings.TrimPrefix(typeName, "*")
	switch typeName {
	case "", "any":
		return nil
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s must be a string", path)
		}
		return nil
	case "bool", "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
		return nil
	case "int", "int64", "integer", "float64", "number":
		switch value.(type) {
		case float64, json.Number:
			return nil
		default:
			return fmt.Errorf("%s must be a number", path)
		}
	}
	if strings.HasPrefix(typeName, "map[string]") {
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		return nil
	}
	model, ok := forgejo.ModelByName(typeName)
	if !ok || len(model.Fields) == 0 {
		return nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s must be an object", path)
	}
	if visiting[typeName] {
		return nil
	}
	visiting[typeName] = true
	defer delete(visiting, typeName)
	knownFields := make(map[string]bool, len(model.Fields))
	for _, field := range model.Fields {
		knownFields[field.Name] = true
	}
	if !model.AdditionalProperties {
		for name := range object {
			if !knownFields[name] {
				return fmt.Errorf("%s.%s is not declared by %s", path, name, typeName)
			}
		}
	}
	for _, field := range model.Fields {
		fieldValue, present := object[field.Name]
		fieldPath := path + "." + field.Name
		if field.Required && (!present || fieldValue == nil) {
			return fmt.Errorf("%s is required", fieldPath)
		}
		if !present || fieldValue == nil {
			continue
		}
		if len(field.Enum) != 0 {
			actual := scalarText(fieldValue)
			if !slices.Contains(field.Enum, actual) {
				return fmt.Errorf("%s must be one of %s, got %q", fieldPath, strings.Join(field.Enum, ", "), actual)
			}
		}
		if field.HasMinimum {
			var number float64
			switch actual := fieldValue.(type) {
			case float64:
				number = actual
			case json.Number:
				parsed, err := actual.Float64()
				if err != nil {
					return fmt.Errorf("%s must be a number", fieldPath)
				}
				number = parsed
			default:
				return fmt.Errorf("%s must be a number", fieldPath)
			}
			if number < field.Minimum {
				return fmt.Errorf("%s must be at least %s", fieldPath, strconv.FormatFloat(field.Minimum, 'g', -1, 64))
			}
		}
		if err := validateSchemaValue(field.Type, fieldValue, fieldPath, visiting); err != nil {
			return err
		}
	}
	return nil
}

func scalarText(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	b, _ := json.Marshal(value)
	return string(b)
}
