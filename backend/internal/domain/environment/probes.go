package environment

import "strings"

func (p *Probe) Validate(singleSuccess bool) error {
	if p == nil {
		return nil
	}
	switch p.Type {
	case "", "http":
		if !strings.HasPrefix(p.Path, "/") || strings.ContainsAny(p.Path, "\x00\r\n") || p.Port < 1 || p.Port > 65535 || len(p.Command) > 0 || (p.Scheme != "" && p.Scheme != "HTTP" && p.Scheme != "HTTPS") {
			return ErrExecution
		}
	case "tcp":
		if p.Port < 1 || p.Port > 65535 || p.Path != "" || len(p.Command) > 0 || p.Scheme != "" {
			return ErrExecution
		}
	case "exec":
		if len(p.Command) == 0 || len(p.Command) > 64 || p.Port != 0 || p.Path != "" || p.Scheme != "" {
			return ErrExecution
		}
		for _, arg := range p.Command {
			if arg == "" || strings.ContainsRune(arg, 0) {
				return ErrExecution
			}
		}
	default:
		return ErrExecution
	}
	if p.InitialDelaySeconds != nil && *p.InitialDelaySeconds < 0 {
		return ErrExecution
	}
	for _, v := range []*int{p.TimeoutSeconds, p.PeriodSeconds, p.SuccessThreshold, p.FailureThreshold} {
		if v != nil && *v < 1 {
			return ErrExecution
		}
	}
	if singleSuccess && p.SuccessThreshold != nil && *p.SuccessThreshold != 1 {
		return ErrExecution
	}
	return nil
}
