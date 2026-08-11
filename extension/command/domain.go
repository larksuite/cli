// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

// DomainName is a generated name of an existing shortcut domain.
type DomainName string

type domainKind uint8

const (
	domainExtended domainKind = iota + 1
	domainNew
)

// Domain is an opaque declaration of where a command set is mounted.
type Domain struct {
	kind    domainKind
	name    string
	options []DomainOption
}

// DomainOption is an opaque property declaration reserved for future new domains.
type DomainOption struct {
	kind  domainOptionKind
	lang  string
	value string
}

type domainOptionKind uint8

const (
	domainTitle domainOptionKind = iota + 1
	domainDescription
)

// ExtendDomain declares that a set adds commands to an existing domain.
func ExtendDomain(name DomainName) Domain {
	return Domain{kind: domainExtended, name: string(name)}
}

// NewDomain fixes the future new-domain API shape. V1 host compilation rejects it.
func NewDomain(name string, opts ...DomainOption) Domain {
	return Domain{kind: domainNew, name: name, options: append([]DomainOption(nil), opts...)}
}

// Title declares one localized title for a future new domain.
func Title(lang, s string) DomainOption {
	return DomainOption{kind: domainTitle, lang: lang, value: s}
}

// Description declares one localized description for a future new domain.
func Description(lang, s string) DomainOption {
	return DomainOption{kind: domainDescription, lang: lang, value: s}
}

// Set groups commands mounted into one domain.
type Set struct {
	_        struct{}
	Domain   Domain
	Commands []Command
}
