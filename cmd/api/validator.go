package main

import (
	"regexp"
)

var EmailRegexp = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

type Validator struct {
	violations map[string]string
}

func NewValidator() *Validator {
	return &Validator{
		violations: make(map[string]string),
	}
}

func (v *Validator) Check(cond bool, key, val string) {
	if cond {
		return
	}
	if _, ok := v.violations[key]; !ok {
		v.violations[key] = val
	}
}

func (v *Validator) CheckUsername(name *string) {
	v.Check(name != nil, "name", "must be provided")
	if name != nil {
		v.Check(*name != "", "name", "must be provided")
		v.Check(len(*name) <= 50, "name", "must not be more than 50 characters")
	}
}

func (v *Validator) CheckEmail(email *string) {
	v.Check(email != nil, "email", "must be provided")
	if email != nil {
		v.Check(*email != "", "email", "must be provided")
		v.Check(EmailRegexp.Match([]byte(*email)), "email", "must be valid")
	}
}

func (v *Validator) CheckPassword(password *string) {
	v.Check(password != nil, "password", "must be provided")
	if password != nil {
		v.Check(*password != "", "password", "must be provided")
		v.Check(len(*password) >= 8, "password", "must be atleast 8 characters")
	}
}

func (v *Validator) HasErrors() bool {
	return len(v.violations) != 0
}
