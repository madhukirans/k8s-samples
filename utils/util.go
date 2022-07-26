package utils

import (
	"encoding/json"
	"fmt"

	//"github.com/infacloud/DormantK8SClusterNotify"
	"github.com/sirupsen/logrus"
	"runtime"
	"strings"
	"sync"
	"time"
)

var singletonLog *logrus.Logger
var once sync.Once

const (
	// Default log format will output [INFO]: 2006-01-02T15:04:05Z07:00 - Log message
	defaultLogFormat       = "time=\"%time%\" level=%lvl% msg=\"%msg%\""
	defaultTimestampFormat = time.RFC3339
)

// Formatter implements logrus.Formatter interface.
type Formatter struct {
	//logrus.TextFormatter
	// Timestamp format
	TimestampFormat string
	// Available standard keys: time, msg, lvl
	// Also can include custom fields but limited to strings.
	// All of fields need to be wrapped inside %% i.e %time% %msg%
	LogFormat string

	CallerPrettyfier func(*runtime.Frame) (function string, file string)
}

// Format building log message.
func (f *Formatter) Format(entry *logrus.Entry) ([]byte, error) {
	output := f.LogFormat
	if output == "" {
		output = defaultLogFormat
	}

	timestampFormat := f.TimestampFormat
	if timestampFormat == "" {
		timestampFormat = defaultTimestampFormat
	}

	for k, v := range entry.Data {
		output = fmt.Sprintf("%s %s=\"%s\"", output, k, v)
	}
	output = strings.Replace(output, "%time%", entry.Time.Format(timestampFormat), 1)
	output = strings.Replace(output, "%msg%", entry.Message, 1)
	level := strings.ToUpper(entry.Level.String())
	output = strings.Replace(output, "%lvl%", strings.ToLower(level), 1)

	var funcVal, fileVal string
	if entry.HasCaller() {
		if f.CallerPrettyfier != nil {
			funcVal, fileVal = f.CallerPrettyfier(entry.Caller)
		} else {
			funcVal = entry.Caller.Function
			fileVal = fmt.Sprintf("%s:%d", entry.Caller.File, entry.Caller.Line)
		}

		if funcVal != "" {
			output = fmt.Sprintf("%s func=\"%s\"", output, funcVal)
		}
		if fileVal != "" {
			output = fmt.Sprintf("%s file=\"%s\"", output, fileVal)
		}
	}

	output = fmt.Sprintf("%s\n", output)
	return []byte(output), nil
}

func GetLogger() *logrus.Logger {
	once.Do(func() {
		singletonLog = logrus.New()
		fmt.Println("DormantK8SClusterNotify logger initiated. This should be called only once.")
		if true {
			singletonLog.Level = logrus.DebugLevel
			singletonLog.SetReportCaller(true)
			singletonLog.Formatter = &Formatter{
				CallerPrettyfier: func(f *runtime.Frame) (string, string) {
					filename1 := strings.Split(f.File, "DormantK8SClusterNotify")
					if len(filename1) > 1 {
						return fmt.Sprintf("%s()", f.Function), fmt.Sprintf("%s:%d", filename1[1], f.Line)
					}

					return fmt.Sprintf("%s()", f.Function), fmt.Sprintf("%s:%d", f.File, f.Line)
				},
			}
		} else {
			singletonLog.Formatter = &Formatter{}
		}
	})

	return singletonLog
}

func GetJsonStr(obj interface{}) string {
	b, err := json.Marshal(obj)
	if err != nil {
		return ""
	}
	return string(b)
}