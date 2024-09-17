package lib

import (
	"fmt"
	"strconv"
)

func castParseInt(input any) (int, error) {
	switch v := input.(type) {
	case string:
		return strconv.Atoi(v)
	case *string:
		return strconv.Atoi(*v)
	case []byte:
		return strconv.Atoi(BytesToString(v))
	}
	return 0, fmt.Errorf("Unsupported parse format")
}

func castParseInt64(input any) (int64, error) {
	// slower than castParseInt. use only if needed
	switch v := input.(type) {
	case string:
		return strconv.ParseInt(v, 10, 64)
	case *string:
		return strconv.ParseInt(*v, 10, 64)
	case []byte:
		return strconv.ParseInt(BytesToString(v), 10, 64)
	}
	return 0, fmt.Errorf("Unsupported parse format")
}

func castParseUint64(input any) (uint64, error) {
	switch v := input.(type) {
	case string:
		return strconv.ParseUint(v, 10, 64)
	case *string:
		return strconv.ParseUint(*v, 10, 64)
	case []byte:
		return strconv.ParseUint(BytesToString(v), 10, 64)
	}
	return 0, fmt.Errorf("Unsupported parse format")
}

func castParseFloat64(input any) (float64, error) {
	switch v := input.(type) {
	case string:
		return strconv.ParseFloat(v, 64)
	case *string:
		return strconv.ParseFloat(*v, 64)
	case []byte:
		return strconv.ParseFloat(BytesToString(v), 64)
	}
	return 0, fmt.Errorf("Unsupported parse format")
}

func castParseBool(input any) (bool, error) {
	switch v := input.(type) {
	case string:
		return strconv.ParseBool(v)
	case *string:
		return strconv.ParseBool(*v)
	case []byte:
		return strconv.ParseBool(BytesToString(v))
	}
	return false, fmt.Errorf("Unsupported parse format")
}

func castToInt64(input any) int64 {
	switch v := input.(type) {
	case int8:
		return int64(v)
	case uint8:
		return int64(v)
	case int16:
		return int64(v)
	case uint16:
		return int64(v)
	case int32:
		return int64(v)
	case uint32:
		return int64(v)
	case int64:
		return int64(v)
	case uint64:
		return int64(v)
	case int:
		return int64(v)
	case uint:
		return int64(v)
	case float32:
		return int64(v)
	case float64:
		return int64(v)
	}
	return 0
}

func castToUint64(input any) uint64 {
	switch v := input.(type) {
	case int8:
		return uint64(v)
	case uint8:
		return uint64(v)
	case int16:
		return uint64(v)
	case uint16:
		return uint64(v)
	case int32:
		return uint64(v)
	case uint32:
		return uint64(v)
	case int64:
		return uint64(v)
	case uint64:
		return uint64(v)
	case int:
		return uint64(v)
	case uint:
		return uint64(v)
	case float32:
		return uint64(v)
	case float64:
		return uint64(v)
	}
	return 0
}

func castToFloat64(input any) float64 {
	switch v := input.(type) {
	case int8:
		return float64(v)
	case uint8:
		return float64(v)
	case int16:
		return float64(v)
	case uint16:
		return float64(v)
	case int32:
		return float64(v)
	case uint32:
		return float64(v)
	case int64:
		return float64(v)
	case uint64:
		return float64(v)
	case int:
		return float64(v)
	case uint:
		return float64(v)
	case float32:
		return float64(v)
	case float64:
		return float64(v)
	}
	return 0
}

func Cast[T ~int | ~uint | ~float64 | ~bool | ~string | ~[]byte | *int | *uint | *float64 | *bool | *string | *[]byte](input any) (ret T) {
	switch any(ret).(type) {
	case string:
		switch v := input.(type) {
		case []byte:
			return any(BytesToString(v)).(T)
		case string:
			return any(v).(T)
		case *string:
			if v == nil {
				return
			}
			return any(*v).(T)
		default:
			return any(fmt.Sprintf("%v", v)).(T)
		}
	case []byte:
		switch v := input.(type) {
		case []byte:
			return any(v).(T)
		case string:
			return any(StringToBytes(v)).(T)
		case *string:
			if v == nil {
				return
			}
			return any(StringToBytes(*v)).(T)
		default:
			return any(StringToBytes(fmt.Sprintf("%v", v))).(T)
		}
	case int:
		switch v := input.(type) {
		case int8, uint8, int16, uint16, int32, uint32, int64, uint64, int, uint, float32, float64:
			return any(int(castToInt64(v))).(T)
		case string, *string, []byte:
			t, err := castParseInt(v)
			if err != nil {
				return
			}
			return any(t).(T)
		}
	case uint:
		switch v := input.(type) {
		case int8, uint8, int16, uint16, int32, uint32, int64, uint64, int, uint, float32, float64:
			return any(uint(castToUint64(v))).(T)
		case string, *string, []byte:
			t, err := castParseUint64(v)
			if err != nil {
				return
			}
			t1 := uint(t)
			return any(t1).(T)
		}
	case float64:
		switch v := input.(type) {
		case int8, uint8, int16, uint16, int32, uint32, int64, uint64, int, uint, float32, float64:
			return any(float64(castToFloat64(v))).(T)
		case string, *string, []byte:
			t, err := castParseFloat64(v)
			if err != nil {
				return
			}
			return any(t).(T)
		}
	case bool:
		switch v := input.(type) {
		case bool:
			return any(v).(T)
		case string, *string, []byte:
			t, err := castParseBool(v)
			if err != nil {
				return
			}
			return any(t).(T)
		}
	case *int:
		switch v := input.(type) {
		case int8, uint8, int16, uint16, int32, uint32, int64, uint64, int, uint, float32, float64:
			t := int(castToInt64(v))
			return any(&t).(T)
		case string, *string, []byte:
			t, err := castParseInt(v)
			if err != nil {
				return any((*int)(nil)).(T)
			}
			return any(&t).(T)
		}
	case *uint:
		switch v := input.(type) {
		case int8, uint8, int16, uint16, int32, uint32, int64, uint64, int, uint, float32, float64:
			t := uint(castToUint64(v))
			return any(&t).(T)
		case string, *string, []byte:
			t, err := castParseUint64(v)
			if err != nil {
				return any((*uint)(nil)).(T)
			}
			t1 := uint(t)
			return any(&t1).(T)
		}
	case *float64:
		switch v := input.(type) {
		case int8, uint8, int16, uint16, int32, uint32, int64, uint64, int, uint, float32, float64:
			t := float64(castToFloat64(v))
			return any(&t).(T)
		case string, *string, []byte:
			t, err := castParseFloat64(v)
			if err != nil {
				return any((*float64)(nil)).(T)
			}
			return any(&t).(T)
		}
	case *bool:
		switch v := input.(type) {
		case bool:
			return any(&v).(T)
		case string, *string, []byte:
			t, err := castParseBool(v)
			if err != nil {
				return any((*bool)(nil)).(T)
			}
			return any(&t).(T)
		}
	case *string:
		switch v := input.(type) {
		case []byte:
			t := BytesToString(v)
			return any(&t).(T)
		case string:
			return any(&v).(T)
		case *string:
			if v == nil {
				return any((*string)(nil)).(T)
			}
			return any(v).(T)
		default:
			t := fmt.Sprintf("%v", v)
			return any(&t).(T)
		}
	case *[]byte:
		switch v := input.(type) {
		case []byte:
			return any(&v).(T)
		case string:
			t := StringToBytes(v)
			return any(&t).(T)
		case *string:
			if v == nil {
				return any((*[]byte)(nil)).(T)
			}
			t := StringToBytes(*v)
			return any(&t).(T)
		default:
			t := StringToBytes(fmt.Sprintf("%v", v))
			return any(&t).(T)
		}
	}

	return
}
