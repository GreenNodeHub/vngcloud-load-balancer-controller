/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package logging

import (
	"path"
	"runtime"
	"strconv"

	"github.com/sirupsen/logrus"
)

// SetupLogger configures the global logrus logger with the specified log level.
// For debug level, timestamps are disabled and a simplified format is used.
// For other levels, timestamps are enabled with file:line caller information.
func SetupLogger(levelStr string) error {
	level, err := logrus.ParseLevel(levelStr)
	if err != nil {
		level = logrus.InfoLevel
	}

	logrus.SetLevel(level)
	logrus.SetReportCaller(true)

	if level == logrus.DebugLevel {
		logrus.SetFormatter(&logrus.TextFormatter{
			DisableTimestamp: true,
			CallerPrettyfier: func(frame *runtime.Frame) (function string, file string) {
				fileName := " " + path.Base(frame.File) + ":" + strconv.Itoa(frame.Line) + " |"
				return "", fileName
			},
		})
	} else {
		logrus.SetFormatter(&logrus.TextFormatter{
			DisableTimestamp: false,
			CallerPrettyfier: func(frame *runtime.Frame) (function string, file string) {
				fileName := path.Base(frame.File) + ":" + strconv.Itoa(frame.Line)
				return "", fileName
			},
		})
	}

	return err
}
