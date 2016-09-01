# Screwdriver Log Service
[![Build Status][wercker-image]][wercker-url]
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
[wercker-image]: https://app.wercker.com/status/be5283837a05b62c65f63fcd90d306a6
[wercker-url]: https://app.wercker.com/project/byKey/be5283837a05b62c65f63fcd90d306a6
[goreport-image]: https://goreportcard.com/badge/github.com/screwdriver-cd/log-service
[goreport-url]: https://goreportcard.com/report/github.com/screwdriver-cd/log-service
