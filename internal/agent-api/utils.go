package agentapi

// returns the given string or nil if empty
func StringOrNil(str string) *string {
	if str == "" {
		return nil
	}
	return &str
}
