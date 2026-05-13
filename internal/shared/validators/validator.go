package validators

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func ValidateStruct(s interface{}) error {
	val := reflect.ValueOf(s)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return errors.New("input must be a struct")
	}

	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		fieldValue := val.Field(i)
		validateTag := field.Tag.Get("validate")

		if validateTag == "" || validateTag == "-" {
			continue
		}

		rules := strings.Split(validateTag, ",")
		fieldName := getFieldName(field)

		for _, rule := range rules {
			rule = strings.TrimSpace(rule)
			if err := applyRule(fieldName, fieldValue, rule); err != nil {
				return err
			}
		}
	}

	return nil
}

func getFieldName(field reflect.StructField) string {
	jsonTag := field.Tag.Get("json")
	if jsonTag != "" && jsonTag != "-" {
		return strings.Split(jsonTag, ",")[0]
	}
	return strings.ToLower(field.Name)
}

func applyRule(fieldName string, fieldValue reflect.Value, rule string) error {
	// Se for ponteiro, pega o valor apontado
	isPointer := fieldValue.Kind() == reflect.Ptr
	if isPointer {
		// Se for nil, não valida (campo não enviado)
		if fieldValue.IsNil() {
			return nil
		}
		fieldValue = fieldValue.Elem()
	}

	parts := strings.Split(rule, "=")
	ruleName := parts[0]
	var ruleValue string
	if len(parts) > 1 {
		ruleValue = parts[1]
	}

	// dive: valida elementos dentro de slices/arrays
	if ruleName == "dive" {
		return validateDive(fieldName, fieldValue)
	}

	// omitempty: pula validação se o campo estiver vazio
	if ruleName == "omitempty" {
		if isEmpty(fieldValue) {
			return nil
		}
		return nil
	}

	// Se o campo está vazio e não é required, pula outras validações
	if isEmpty(fieldValue) && ruleName != "required" {
		return nil
	}

	switch ruleName {
	case "required":
		return validateRequired(fieldName, fieldValue)
	case "min":
		return validateMin(fieldName, fieldValue, ruleValue)
	case "max":
		return validateMax(fieldName, fieldValue, ruleValue)
	case "gt":
		return validateGt(fieldName, fieldValue, ruleValue)
	case "email":
		return validateEmail(fieldName, fieldValue)
	case "oneof":
		return validateOneOf(fieldName, fieldValue, ruleValue)
	case "uuid":
		return validateUUID(fieldName, fieldValue)
	default:
		return nil
	}
}

func isEmpty(fieldValue reflect.Value) bool {
	switch fieldValue.Kind() {
	case reflect.String:
		return strings.TrimSpace(fieldValue.String()) == ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fieldValue.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fieldValue.Uint() == 0
	case reflect.Bool:
		return !fieldValue.Bool()
	case reflect.Slice, reflect.Map, reflect.Array:
		return fieldValue.Len() == 0
	}
	return false
}

func validateRequired(fieldName string, fieldValue reflect.Value) error {
	switch fieldValue.Kind() {
	case reflect.String:
		if strings.TrimSpace(fieldValue.String()) == "" {
			return fmt.Errorf("%s is required", fieldName)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if fieldValue.Int() == 0 {
			return fmt.Errorf("%s is required", fieldName)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if fieldValue.Uint() == 0 {
			return fmt.Errorf("%s is required", fieldName)
		}
	}
	return nil
}

func validateMin(fieldName string, fieldValue reflect.Value, minStr string) error {
	min, err := strconv.Atoi(minStr)
	if err != nil {
		return fmt.Errorf("invalid min value for %s", fieldName)
	}

	switch fieldValue.Kind() {
	case reflect.String:
		str := strings.TrimSpace(fieldValue.String())
		if str != "" && len(str) < min {
			return fmt.Errorf("%s must be at least %d characters", fieldName, min)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if fieldValue.Int() < int64(min) {
			return fmt.Errorf("%s must be at least %d", fieldName, min)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if fieldValue.Uint() < uint64(min) {
			return fmt.Errorf("%s must be at least %d", fieldName, min)
		}
	}
	return nil
}

func validateMax(fieldName string, fieldValue reflect.Value, maxStr string) error {
	max, err := strconv.Atoi(maxStr)
	if err != nil {
		return fmt.Errorf("invalid max value for %s", fieldName)
	}

	switch fieldValue.Kind() {
	case reflect.String:
		str := strings.TrimSpace(fieldValue.String())
		if str != "" && len(str) > max {
			return fmt.Errorf("%s must be at most %d characters", fieldName, max)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if fieldValue.Int() > int64(max) {
			return fmt.Errorf("%s must be at most %d", fieldName, max)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if fieldValue.Uint() > uint64(max) {
			return fmt.Errorf("%s must be at most %d", fieldName, max)
		}
	}
	return nil
}

func validateGt(fieldName string, fieldValue reflect.Value, gtStr string) error {
	gt, err := strconv.Atoi(gtStr)
	if err != nil {
		return fmt.Errorf("invalid gt value for %s", fieldName)
	}

	switch fieldValue.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if fieldValue.Int() <= int64(gt) {
			return fmt.Errorf("%s must be greater than %d", fieldName, gt)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if fieldValue.Uint() <= uint64(gt) {
			return fmt.Errorf("%s must be greater than %d", fieldName, gt)
		}
	}
	return nil
}

func validateEmail(fieldName string, fieldValue reflect.Value) error {
	if fieldValue.Kind() != reflect.String {
		return fmt.Errorf("%s must be a string", fieldName)
	}

	email := strings.TrimSpace(fieldValue.String())
	if email != "" && !emailRegex.MatchString(email) {
		return fmt.Errorf("%s has invalid email format", fieldName)
	}

	return nil
}

func validateOneOf(fieldName string, fieldValue reflect.Value, allowedValues string) error {
	if fieldValue.Kind() != reflect.String {
		return fmt.Errorf("%s must be a string", fieldName)
	}

	value := strings.TrimSpace(fieldValue.String())
	if value == "" {
		return nil
	}

	allowed := strings.Split(allowedValues, " ")
	for _, allowedValue := range allowed {
		if value == strings.TrimSpace(allowedValue) {
			return nil
		}
	}

	return fmt.Errorf("%s must be one of: %s", fieldName, strings.Join(allowed, ", "))
}

func validateUUID(fieldName string, fieldValue reflect.Value) error {
	if fieldValue.Kind() != reflect.String {
		return fmt.Errorf("%s must be a string", fieldName)
	}

	uuid := strings.TrimSpace(fieldValue.String())
	if uuid == "" {
		return fmt.Errorf("%s is required", fieldName)
	}

	// UUID v4 format: 8-4-4-4-12 hexadecimal characters
	uuidRegex := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	if !uuidRegex.MatchString(uuid) {
		return fmt.Errorf("%s must be a valid UUID format", fieldName)
	}

	return nil
}

func validateDive(fieldName string, fieldValue reflect.Value) error {
	// dive is used to validate elements inside slices/arrays
	if fieldValue.Kind() != reflect.Slice && fieldValue.Kind() != reflect.Array {
		return nil
	}

	for i := 0; i < fieldValue.Len(); i++ {
		elem := fieldValue.Index(i)

		// Se o elemento for um ponteiro, pega o valor apontado
		if elem.Kind() == reflect.Ptr {
			if elem.IsNil() {
				continue
			}
			elem = elem.Elem()
		}

		// Valida structs recursivamente
		if elem.Kind() == reflect.Struct {
			if err := ValidateStruct(elem.Addr().Interface()); err != nil {
				return fmt.Errorf("%s[%d]: %w", fieldName, i, err)
			}
		}
	}

	return nil
}

func ValidateID(idStr string) (uint, error) {
	if strings.TrimSpace(idStr) == "" {
		return 0, errors.New("id is required")
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return 0, errors.New("invalid id format")
	}

	if id == 0 {
		return 0, errors.New("id must be greater than 0")
	}

	return uint(id), nil
}

func ValidateQueryParams(params map[string]string) error {
	if limit, ok := params["limit"]; ok {
		val, err := strconv.Atoi(limit)
		if err != nil {
			return errors.New("limit must be a valid number")
		}
		if val < 1 || val > 1000 {
			return errors.New("limit must be between 1 and 1000")
		}
	}

	if offset, ok := params["offset"]; ok {
		val, err := strconv.Atoi(offset)
		if err != nil {
			return errors.New("offset must be a valid number")
		}
		if val < 0 {
			return errors.New("offset must be greater than or equal to 0")
		}
	}

	return nil
}
