package lib

import (
	"os"
	"reflect"
	"strconv"
	"strings"
)

func setValue(field reflect.Value, typ reflect.Type, source string, sourceKey string, defaultValue string) {
	if !field.CanSet() {
		return
	}

	var value string
	kind := typ.Kind()

	if strings.ToLower(os.Getenv("LOCAL")) == "true" {
		source = "env"
	}

	if source == "sec" {
		if sourceKey[0] != '/' {
			sourceKey = "/var/secrets/" + sourceKey // alloc
		}

		f, err := os.ReadFile(sourceKey)
		if err != nil {
			panic(err)
		}
		value = BytesToString(f)
	} else {
		value = os.Getenv(sourceKey)
	}

	if value == "" {
		value = defaultValue
	}

	switch kind {
	case reflect.Int8:
		v, e := strconv.ParseInt(value, 10, 8)
		if e == nil {
			field.SetInt(v)
		}
	case reflect.Int16:
		v, e := strconv.ParseInt(value, 10, 16)
		if e == nil {
			field.SetInt(v)
		}
	case reflect.Int32:
		v, e := strconv.ParseInt(value, 10, 32)
		if e == nil {
			field.SetInt(v)
		}
	case reflect.Int:
		v, e := strconv.ParseInt(value, 10, 64)
		if e == nil {
			field.SetInt(v)
		}
	case reflect.Int64:
		v, e := strconv.ParseInt(value, 10, 64)
		if e == nil {
			field.SetInt(v)
		}
	case reflect.Bool:
		v, e := strconv.ParseBool(value)
		if e == nil {
			field.SetBool(v)
		}
	case reflect.Float32:
		v, e := strconv.ParseFloat(value, 32)
		if e == nil {
			field.SetFloat(v)
		}
	case reflect.Float64:
		v, e := strconv.ParseFloat(value, 64)
		if e == nil {
			field.SetFloat(v)
		}
	case reflect.String:
		field.SetString(value)
	case reflect.Slice:
		if _, ok := field.Interface().([]byte); ok {
			field.SetBytes(StringToBytes(value))
		}
	}
}

// LoadConfig reads config from ENV / secret volume
//
// Example:
//
//	type Config struct {
//		MongoConnStr      string `env:"MONGO_CONN_STR"`
//		SomeFeatureToggle bool   `env:"ENABLE_FOO" default:"true"`
//		ByteArr           []byte `sec:"SECRET_NAME"`
//	}
//
//	config := lib.LoadConfig()
func LoadConfig[T any]() *T {
	config := new(T)
	typ := reflect.TypeOf(*config)

	value := reflect.ValueOf(config).Elem()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		sourceKey, source := field.Tag.Get("env"), "env"
		if sourceKey == "" {
			sourceKey, source = field.Tag.Get("sec"), "sec"
		}

		defaultValue := field.Tag.Get("default")
		setValue(value.Field(i), field.Type, source, sourceKey, defaultValue)
	}

	// Future improvement: handle SIGHUP here.

	return config
}
