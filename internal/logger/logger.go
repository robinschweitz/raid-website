package logger

import "log"

func Info(msg string, args ...any) {
	log.Printf("INFO "+msg, args...)
}
func Error(msg string, args ...any) {
	log.Printf("ERROR "+msg, args...)
}
func Warn(msg string, args ...any) {
	log.Printf("WARN "+msg, args...)
}
func Debug(msg string, args ...any) {
	log.Printf("DEBUG "+msg, args...)
}
