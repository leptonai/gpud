package common

// MatchFunc is a function that matches a line of log and returns a Match.
// It returns empty strings if the line does not match.
type MatchFunc func(line string) (eventName string, regex string, message string)
