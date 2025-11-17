# regreSQL Fixture Examples

Standalone examples designed to help you understand various fixture types and capabilities. Each
ficture tries to demostrate different aspect of the fixture system.

**Getting Started**

Start with creating target database and load the prepared schema.

```bash
createdb regresql_fixtures
psql regresql_fixtures < schema.sql
```

Modify the `pguri` within regresql/regress.yaml to point to your database.

## Fixtures commands

```bash
# List available fixtures
regresql fixtures list
```
