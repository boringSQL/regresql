# PostgreSQL code

Not ours. Everything here is copied from the PostgreSQL tree. It is the base
for the 2.1 backport of the stats restore functions, so 16 and 17 can load a
dump made by pg_dump 18 or 19. The rest of pg_regresql is BSD-2-Clause; these
files stay under the PostgreSQL License.

## Provenance

    repo      https://github.com/postgres/postgres
    branch    REL_18_STABLE
    commit    780900dcfab (2026-11-09)

    src/backend/statistics/relation_stats.c   -> relation_stats.c
    src/backend/statistics/attribute_stats.c  -> attribute_stats.c
    src/backend/statistics/stat_utils.c       -> stat_utils.c
    src/include/statistics/stat_utils.h       -> stat_utils.h

158c761 imported them from 1370a7832a2 (2026-10-07). bf52896 moved them to
780900dcfab. The only upstream change in between is 780900dcfab (2026-11-09),
CVE-2026-116329: hold the target relation open while the attribute input is
parsed, and re-check the column before writing. Nothing else in these four
files changed on REL_18_STABLE in that window.

## Local changes

The port commit is the diff worth reading:

    git diff bf52896..<port-commit> -- pg_ext/restore

Each .c file is wrapped in

    #if PG_VERSION_NUM >= 160000 && PG_VERSION_NUM < 180000

so 14 and 15 keep building (empty objects) and 18+ is unaffected. On 16/17 the
two functions are real.

Every non-static symbol got a regresql_ prefix. It is cheap, and it stops a
server fork that exports the same names from colliding with us.
PG_FUNCTION_INFO_V1 went on the two entry points only.

statistics/stat_utils.h does not exist on 16/17, so the local stat_utils.h is
included instead. On 16 attribute_stats.c also needs utils/typcache.h and
nodes/miscnodes.h (17 and 18 happen to get them transitively), and stat_utils.c
needs catalog/objectaddress.h for get_relkind_objtype.

The ownership check had to be split. ACL_MAINTAIN is 17+, and
pg_class_ownercheck was removed in 16. So 17 uses pg_class_aclcheck(...,
ACL_MAINTAIN), 16 falls back to object_ownercheck(RelationRelationId, ...), and
the database owner bypasses both.

relallfrozen exists only on 18+. The kwarg stays in relarginfo so a dump that
passes it is still accepted and type-checked, but the value is dropped, there is
no column to put it in. That also shrinks the update arrays from 4 to 3.

pg_clear_relation_stats and pg_clear_attribute_stats are gone, together with
cleararginfo and delete_pg_statistic, which only the clear function called.
init_empty_stats_tuple stays because attribute_statistics_update uses it.

Extended statistics are a separate problem and are not ported here.

Otherwise the bodies are upstream, tabs and all.

## Re-syncing

Copy the four files from a newer REL_18_STABLE, commit that as a vendor commit,
then rebase the port commit on top. A plain patch will not apply if upstream
touched the same functions, so let git do a 3-way merge (git apply -3, or
rebase/cherry-pick). Conflicts stay in this directory.

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
