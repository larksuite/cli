// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

// DomainName is the name of an existing Lark business domain.
type DomainName string

type domainKind uint8

const domainExtended domainKind = iota + 1

// Domain is an opaque declaration of where a command set is mounted.
type Domain struct {
	kind domainKind
	name string
}

// ExtendDomain declares that a set adds commands to an existing domain.
// V1 mounts business commands into existing domains only; declaring a brand new
// domain is not part of this surface, so no constructor for one is exported.
func ExtendDomain(name DomainName) Domain {
	return Domain{kind: domainExtended, name: string(name)}
}

// Set groups commands mounted into one domain.
type Set struct {
	_        struct{}
	Domain   Domain
	Commands []Command
}
