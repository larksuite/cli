// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

// DomainName is the name of a Lark business domain: the root command a set's
// commands mount under, such as im in `lark-cli im +chat-brief`.
type DomainName string

type domainKind uint8

const (
	domainExtended domainKind = iota + 1
	domainNew
)

// Domain is an opaque declaration of where a command set is mounted.
type Domain struct {
	kind        domainKind
	name        string
	description string
}

// ExtendDomain declares that a set adds commands to a domain lark-cli ships
// (one of the Domain* constants). A domain declared with NewDomain is owned by
// its own set and cannot be extended.
func ExtendDomain(name DomainName) Domain {
	return Domain{kind: domainExtended, name: string(name)}
}

// DomainSpec describes a business domain that lark-cli does not ship.
type DomainSpec struct {
	// Name is the root command the set's commands mount under, so every command
	// in the set declares it as Metadata.Service. It must match
	// ^[a-z][a-z0-9]*(-[a-z0-9]+)*$ and may not reuse a built-in domain or one
	// of its aliases, a lark-cli root command, or the `auth login --domain`
	// keyword `all`. Prefix it with the distribution's name: a later lark-cli
	// release that ships a built-in domain of the same name refuses to start
	// this build.
	Name DomainName
	// Description is the one-line summary shown next to the domain in
	// `lark-cli --help` and as the lead of `lark-cli <name> --help`.
	Description string

	_ struct{}
}

// NewDomain declares a domain owned entirely by the set that carries it. The
// host mounts it as a root command grouped with the Lark domains, accepts it in
// `auth login --domain`, tags its commands for policy rules and hook selectors,
// and reads the `affordance/<name>.md` and skills the distribution ships for
// its guidance. One build declares a given name at most once.
func NewDomain(spec DomainSpec) Domain {
	return Domain{kind: domainNew, name: string(spec.Name), description: spec.Description}
}

// Set groups commands mounted into one domain.
type Set struct {
	_        struct{}
	Domain   Domain
	Commands []Command
}
