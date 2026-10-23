# PostgreSQL upstream code

The files in this directory are **not original RegreSQL code**. They are copied
from the PostgreSQL source tree as the base for the pg_regresql 2.1 backport of
the statistics-restore functions to PostgreSQL 16 and 17.

## Provenance

| Item | Value |
|---|---|
| Upstream repository | https://github.com/postgres/postgres |
| Branch | `REL_18_STABLE` |
| Commit | `1370a7832a2ab7bda5625fd1f0448808b534cd50` |
| Commit date | 2026-10-07 |
| Files | `src/backend/statistics/relation_stats.c` |
| | `src/backend/statistics/attribute_stats.c` |
| | `src/backend/statistics/stat_utils.c` |
| | `src/include/statistics/stat_utils.h` |
| License | PostgreSQL License (below) |

The import commit is verbatim upstream content plus a provenance comment at the
top of each file. Later commits add the local port changes described in
`PLAN-stats-pre18.md`: symbol prefixing, the local `stat_utils.h` include, the
`#if PG_VERSION_NUM < 180000` guard, and the `relallfrozen` / `ACL_MAINTAIN`
shims.

To re-sync, diff this directory against a newer `REL_18_STABLE` commit and keep
the local changes as a small, documented patch.

## License

The copied files retain their upstream copyright notices and are distributed
under the PostgreSQL License. The rest of pg_regresql remains under its
BSD 2-Clause license.

```
PostgreSQL Database Management System
(also known as Postgres, formerly known as Postgres95)

Portions Copyright (c) 1996-2026, PostgreSQL Global Development Group

Portions Copyright (c) 1994, The Regents of the University of California

Permission to use, copy, modify, and distribute this software and its
documentation for any purpose, without fee, and without a written agreement
is hereby granted, provided that the above copyright notice and this
paragraph and the following two paragraphs appear in all copies.

IN NO EVENT SHALL THE UNIVERSITY OF CALIFORNIA BE LIABLE TO ANY PARTY FOR
DIRECT, INDIRECT, SPECIAL, INCIDENTAL, OR CONSEQUENTIAL DAMAGES, INCLUDING
LOST PROFITS, ARISING OUT OF THE USE OF THIS SOFTWARE AND ITS
DOCUMENTATION, EVEN IF THE UNIVERSITY OF CALIFORNIA HAS BEEN ADVISED OF THE
POSSIBILITY OF SUCH DAMAGE.

THE UNIVERSITY OF CALIFORNIA SPECIFICALLY DISCLAIMS ANY WARRANTIES,
INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY
AND FITNESS FOR A PARTICULAR PURPOSE.  THE SOFTWARE PROVIDED HEREUNDER IS
ON AN "AS IS" BASIS, AND THE UNIVERSITY OF CALIFORNIA HAS NO OBLIGATIONS TO
PROVIDE MAINTENANCE, SUPPORT, UPDATES, ENHANCEMENTS, OR MODIFICATIONS.
```
