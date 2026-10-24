# TOKEN engine

This repository is the complete source code of the software that runs
[TOKEN](https://thetoken.dev): the API at `https://api.thetoken.dev` and the
console at `https://app.thetoken.dev`. It is a modified version of
[New API](https://github.com/QuantumNous/new-api) by QuantumNous and
contributors, published under the GNU Affero General Public License v3.0
(see [LICENSE](LICENSE) and [NOTICE](NOTICE)).

TOKEN is an independent service built on the open-source New API project. It
is not affiliated with or endorsed by QuantumNous.

## Which version is running

Each TOKEN release is a tag here with the same name as the deployed release
(for example `v1.0.0-8fd3daeb9880`). The console shows the running release in
its version label. The tag holds exactly the source of that release.

## What changed relative to New API

TOKEN's changes are based on New API tag `v1.0.0-rc.42`. Every added or
changed file, with the date of its latest change, is listed in
[TOKEN_MODIFICATIONS.md](TOKEN_MODIFICATIONS.md). In short: Moyasar (SAR)
top-ups with a purchase fee, a starter credit, hashed API keys and key
rotation, admin MFA, sign-up limits, English log text, the provider name in
usage logs, public aggregate metrics, data retention, a stream header timeout,
security headers, and a rebranded console.

## Building

See [BUILDING.md](BUILDING.md).

## What is not here

The operator's data and deployment are not part of the program and are not
published: the model catalog and prices, routing choices, console settings,
infrastructure and deployment scripts, the marketing website, and secrets. The
program runs without them; an operator configures channels, prices and options
through the console or its admin API, as with New API.

## Source requests and contact

support@thetoken.dev
