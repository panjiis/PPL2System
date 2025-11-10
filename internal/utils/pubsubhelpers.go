package utils

import (
	"fmt"
	"time"
)

func GenerateEventID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().Unix())
}
