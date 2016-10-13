# Screwdriver Log Service
[![Build Status][status-image]][status-url]
[![Open Issues][issues-image]][issues-url]
[![Go Report Card][goreport-image]][goreport-url]

> Sidecar for reading logs from the Screwdriver Launcher and uploading to the Screwdriver Store

## Usage

```bash
$ go get github.com/screwdriver-cd/log-service
$ logservice cba94a05f8aa063f4b8cfb62cbc355e0c5f02698
```

## Testing

```bash
$ go get github.com/screwdriver-cd/log-service
$ go test -cover github.com/screwdriver-cd/log-service/...
```

## License

Code licensed under the BSD 3-Clause license. See LICENSE file for terms.

[issues-image]: https://img.shields.io/github/issues/screwdriver-cd/log-service.svg
[issues-url]: https://github.com/screwdriver-cd/log-service/issues
[status-image]: https://cd.screwdriver.cd/pipelines/c0a9fd6a591c991e07f632c5ffe43c132b547de9/badge
[status-url]: https://cd.screwdriver.cd/pipelines/c0a9fd6a591c991e07f632c5ffe43c132b547de9
[goreport-image]: https://goreportcard.com/badge/github.com/screwdriver-cd/log-service
[goreport-url]: https://goreportcard.com/report/github.com/screwdriver-cd/log-service
