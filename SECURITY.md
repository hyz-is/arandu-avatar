# Security policy

## Supported versions

The latest minor release of Arandu Avatar receives security fixes. Older minor
releases do not: a published Go module version is immutable, so a fix is a new
release rather than a moved tag.

## Reporting a vulnerability

Report it privately, through GitHub's advisory form:

<https://github.com/hyz-is/arandu-avatar/security/advisories/new>

Do not open a public issue and do not describe the problem in a pull request.
A report that arrives in public is a report every reader of this repository can
act on before there is a release to upgrade to.

Include what you need to reproduce it: the version, the wiring, and the request
or call that triggers it.

Expect an acknowledgement within a few days. If the report is confirmed, the
fix, the release and the advisory are published together, and you are credited
unless you ask not to be.

## What is in scope

Anything that lets a caller reach data a policy did not authorize. In
particular:

- a path from a handler to a Model or database handle that does not pass through
  the Service and its Policy;
- a route reachable without `RequireAuth`, or a handler that takes who is
  asking from anywhere but the subject the guard put on the request;
- a Model terminal reached before `security.Authorize`;
- a query whose tenant scope is disabled, or a tenant taken from anywhere but
  `data.Tenant(g)`;
- a `Grant` that can be produced without a policy returning nil;
- a field reaching a response that `Resource` does not list;
- a photo served without the policy having answered, or by a public address of the
  bucket;
- an upload that is accepted by its name or announced type rather than its bytes,
  that is decoded before its declared size is held against the ceiling, or that
  keeps a byte of EXIF or GPS;
- the e-mail, or the digest of it, reaching a log, an activity entry or anything but
  the one request a person asked for when they switched Gravatar on;
- markup of a generated avatar that carries something other than a validated colour,
  a number or escaped text.

## What is not

- A vulnerability in the framework itself. Report that to the framework.
- A policy that is too permissive in an application that opened it. What this
  package ships denies everything; what an application opens is its own.
