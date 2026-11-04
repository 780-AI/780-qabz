<!-- VERSION: 1.0 -->
# 004_use_yml_config — Use a YAML config file

Status: done

## Description

The config format is YAML now. The App only understands the YAML config file passed with the required argument `--config=path/to/config.yml`. If the argument is missing, or the file does not exist or is wrong, the App panics and exits.

The ENV key `POSTGRES_HOST` becomes, in YAML:

```yml
database:
  postgres:
    host: "localhost"
```

A Go type expects the same kind of node from the YAML. These rules must be followed:

| YAML | Go |
|---|---|
| Array | `[]T` |
| Object | `struct`, `map[string]T` |
| Scalar | `int`, `int8`, `int16`, `int32`, `int64`, `uint`, `uint8`, `uint16`, `uint32`, `uint64`, `string`, `time.Duration` |

## Steps

- Update the bootloader.
- Complete the config mapper.
- Update the config tests. Use mock values of a complex YAML file, and check that the custom structs of the tests are mapped correctly.

## Definition of Done

- The App loads its config from the file given with `--config`; a missing or wrong file makes it panic and exit.
- The mapper follows the table above for every type.
- The config tests use a complex YAML mock and pass.
- All tests for configs must pass.
