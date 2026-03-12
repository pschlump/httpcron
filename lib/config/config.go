package config

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
)

// Config contains the configuration of the url shortener.
type Config struct {
	Server struct {
		Host string `json:"host" default:"127.0.0.1"`
		Port string `json:"port" default:"9118"`
	} `json:"server"`
	Debug struct {
		Enabled map[string]bool `json:"enabled"`
	} `json:"debug"`
	Version   string
	BuildDate string
}

// FromFile returns a configuration parsed from the given file.
func FromFile(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	if err = SetDefaults(cfg); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// SetDefaults walks through a struct and sets default values from "default" tags
// The input must be a pointer to a struct
func SetDefaults(v interface{}) error {
	rv := reflect.ValueOf(v)

	// Must be a pointer
	if rv.Kind() != reflect.Ptr {
		return fmt.Errorf("SetDefaults requires a pointer to a struct, got %v", rv.Kind())
	}

	// Must point to a struct
	if rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("SetDefaults requires a pointer to a struct, got pointer to %v", rv.Elem().Kind())
	}

	return setDefaults(rv.Elem())
}

// setDefaults is the recursive worker function
func setDefaults(rv reflect.Value) error {
	rt := rv.Type()

	for i := 0; i < rv.NumField(); i++ {
		field := rv.Field(i)
		fieldType := rt.Field(i)

		// Skip unexported fields
		if !field.CanSet() {
			continue
		}

		// Get the default tag
		defaultTag := fieldType.Tag.Get("default")

		// If field is a struct, recurse into it
		if field.Kind() == reflect.Struct {
			if err := setDefaults(field); err != nil {
				return fmt.Errorf("error setting defaults in field %s: %w", fieldType.Name, err)
			}
			continue
		}

		// If field is a pointer to a struct, initialize and recurse
		if field.Kind() == reflect.Ptr && field.Type().Elem().Kind() == reflect.Struct {
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			if err := setDefaults(field.Elem()); err != nil {
				return fmt.Errorf("error setting defaults in field %s: %w", fieldType.Name, err)
			}
			continue
		}

		// Skip if no default tag
		if defaultTag == "" {
			continue
		}

		// Set the default value based on field type
		if err := setFieldDefault(field, defaultTag, fieldType.Name); err != nil {
			return err
		}
	}

	return nil
}

// setFieldDefault sets a single field's default value
func setFieldDefault(field reflect.Value, defaultValue string, fieldName string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(defaultValue)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		val, err := strconv.ParseInt(defaultValue, 10, 64)
		if err != nil {
			return fmt.Errorf("field %s: invalid int default value '%s': %w", fieldName, defaultValue, err)
		}
		field.SetInt(val)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		val, err := strconv.ParseUint(defaultValue, 10, 64)
		if err != nil {
			return fmt.Errorf("field %s: invalid uint default value '%s': %w", fieldName, defaultValue, err)
		}
		field.SetUint(val)

	case reflect.Float32, reflect.Float64:
		val, err := strconv.ParseFloat(defaultValue, 64)
		if err != nil {
			return fmt.Errorf("field %s: invalid float default value '%s': %w", fieldName, defaultValue, err)
		}
		field.SetFloat(val)

	case reflect.Bool:
		val, err := strconv.ParseBool(defaultValue)
		if err != nil {
			return fmt.Errorf("field %s: invalid bool default value '%s': %w", fieldName, defaultValue, err)
		}
		field.SetBool(val)

	default:
		return fmt.Errorf("field %s: unsupported type %v for default tag", fieldName, field.Kind())
	}

	return nil
}

/* vim: set noai ts=4 sw=4: */
