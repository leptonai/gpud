package customplugins

// Validate validates the plugin step.
func (st *Step) Validate() error {
	if st.Name == "" {
		return ErrStepNameRequired
	}

	switch {
	case st.RunBashScript != nil:
		return st.RunBashScript.Validate()

	default:
		return ErrMissingPluginStep
	}
}
