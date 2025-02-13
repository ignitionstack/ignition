package engine

import "log"

type Logger interface {
	Printf(format string, v ...interface{})
	Errorf(format string, v ...interface{})
	Debugf(format string, v ...interface{})
}

type stdLogger struct {
	*log.Logger
}

func (l *stdLogger) Printf(format string, v ...interface{}) {
	l.Logger.Printf(format, v...)
}

func (l *stdLogger) Errorf(format string, v ...interface{}) {
	l.Logger.Printf("ERROR: "+format, v...)
}

func (l *stdLogger) Debugf(format string, v ...interface{}) {
	l.Logger.Printf("DEBUG: "+format, v...)
}
