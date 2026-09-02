package organize

import "fmt"

func fmtSprintf(format string, v ...interface{}) string {
	return fmt.Sprintf(format, v...)
}
