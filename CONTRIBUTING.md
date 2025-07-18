# Contributing to CEEMS 🌱

Thanks for your interest in contributing to CEEMS! This guide will help
you get started with contributing to our project.

## What Should I Know Before I Get Started? 🤔

CEEMS is a Go-based project focused on sustainable computing. Before contributing,
it's helpful to:

- Have basic knowledge of Go programming
- Understand SLURM, Openstack and Kubernetes
- Familiarize yourself with the project structure by exploring the repository

## Gen AI policy

This project adheres to the Linux Foundation's Generative AI Policy, which can be
viewed at [https://www.linuxfoundation.org/legal/generative-ai](https://www.linuxfoundation.org/legal/generative-ai).

## How Do I Start Contributing? 🚀

1. Fork the repository on GitHub
2. Clone your fork locally
3. Set up your development environment (see below)
4. Create a new branch for your work
5. Make your changes
6. Submit a pull request

<!-- Don't forget to sign your commits according to our [DCO](DCO) requirements! -->

## How Can I Contribute? 💡

### Continuous Integration 🔄

This project uses GitHub Actions for CI. Each PR will trigger automated builds
and tests. Check the `.github/workflows` directory to understand our CI pipeline.
Ensure your contributions pass all CI checks before requesting a review.

### Code Review 👀

All submissions require review. This project use GitHub pull requests for this purpose:

1. Submit a pull request from your fork to our main repository
2. Ensure all CI checks pass
3. Address feedback from maintainers
4. Once approved, a maintainer will merge your changes

### Reporting Bugs 🐛

Found a bug? Please report it by creating an issue with the bug template. Include:

- A clear and descriptive title
- Steps to reproduce the issue
- Expected vs actual behavior
- Screenshots or logs if applicable
- Your environment details (OS, Go version, etc.)

### Suggesting Enhancements ✨

#### How Do I Submit A (Good) Enhancement Suggestion?

Enhancement suggestions are tracked as GitHub issues. Create an issue with
the enhancement template and provide:

- A clear and descriptive title
- A detailed description of the proposed enhancement
- Any potential implementation ideas you have
- Why this enhancement would be useful to most CEEMS users

#### How Do I Submit A (Good) Improvement Item?

For smaller improvements to existing functionality:

- Focus on a single, specific improvement
- Explain how your improvement makes CEEMS better
- Provide context around why this improvement matters
- Link to any related issues or discussions

## Development 💻

### Set up your dev environment

### Prerequisites

- Go (1.24 or later recommended)
- LLVM (to compile eBPF codes)

### Building

The project has a make file which can be used to compile apps. CEEMS ships
different apps of which `ceems_api_server` and `ceems_lb` are CGO based and
the rest are pure Go apps.

To build pure Go apps,

```bash
make build
```

To build CGO apps,

```bash
CGO_APPS=1 make build
```

### Testing

CEEMS apps contains unit and e2e tests which must be passed.

Run unit tests for both Go and CGO apps with:

```bash
make test
CGO_APPS=1 make test
```

Generate coverage reports with:

```bash
make coverage
```

Similarly, run e2e tests for Go and CGO apps with:

```bash
make test-e2e
CGO_APPS=1 make test-e2e
```

e2e tests compare the output of current test with the expected output. If the current
changes made in the source code changes the expected output as well, the expected
output files can be updated with:

```bash
make test-e2e-update
CGO_APPS=1 make test-e2e-update
```

For `ceems_exporter`, unit and e2e tests are run against fake `/sys` and `/proc` file
systems which will be extracted to `pkg/collector/testdata/{sys,proc}` folders, respectively.
If more test resources must be added to these fake file systems, they can be simply copied
into appropriate folders and then archived with:

```bash
./scripts/ttar -C pkg/collector/testdata -c -f pkg/collector/testdata/sys.ttar sys
./scripts/ttar -C pkg/collector/testdata -c -f pkg/collector/testdata/proc.ttar proc
```

## Your First Code Contribution 🎉

Looking for a place to start?

1. Look for issues labeled `good first issue` or `help wanted`
2. Read the code in the area you want to work on to understand patterns
3. Ask questions in issues or discussions if you need help

Remember that even small contributions like fixing typos or improving documentation are valuable!

Thanks for contributing to CEEMS! 💚
