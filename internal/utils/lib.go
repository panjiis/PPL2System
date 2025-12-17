package utils

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

var wibLoc, _ = time.LoadLocation("Asia/Jakarta")

func StrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func StrPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func Int32Ptr(i int32) *int32 {
	return &i
}

func BoolPtr(b bool) *bool {
	return &b
}

type StringArray []string

func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = []string{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to scan StringArray: %v", value)
	}
	return json.Unmarshal(bytes, a)
}

func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return "[]", nil
	}
	return json.Marshal(a)
}

func TimeNowOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func IntToInt32Ptr(i *int) *int32 {
	if i == nil {
		return nil
	}
	v := int32(*i)
	return &v
}

func LocalToWIBTimestamp(in *timestamppb.Timestamp) *timestamppb.Timestamp {
    if in == nil {
        return nil
    }

    t := in.AsTime().In(wibLoc)

    wallClockUTC := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
    return timestamppb.New(wallClockUTC)
}
