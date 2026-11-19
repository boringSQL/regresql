# PostgreSQL code

Not ours. Everything here is copied from the PostgreSQL tree. It is the base
for the 2.1 backport of the stats restore functions and the 2.2 backport of the
extended statistics ones, so 16 and 17 can load a dump made by pg_dump 18 or 19. The rest of pg_regresql is BSD-2-Clause; these
files stay under the PostgreSQL License.

## Provenance

    repo      https://github.com/postgres/postgres
    branch    REL_18_STABLE
    commit    780900dcfab (2026-11-09)

    src/backend/statistics/relation_stats.c   -> relation_stats.c
    src/backend/statistics/attribute_stats.c  -> attribute_stats.c
    src/backend/statistics/stat_utils.c       -> stat_utils.c
    src/include/statistics/stat_utils.h       -> stat_utils.h

2.2 adds files from PostgreSQL master, since 18 has no extended statistics
restore either:

    repo      https://github.com/postgres/postgres
    branch    master
    commit    061065e28f0 (2026-10-08)

    src/backend/statistics/extended_stats_funcs.c  -> extended_stats_funcs.c
    src/backend/utils/adt/pg_ndistinct.c           -> pg_ndistinct.c (input routine only)
    src/backend/utils/adt/pg_dependencies.c        -> pg_dependencies.c (input routine only)
    src/include/statistics/statistics_format.h     -> statistics_format.h (verbatim)
    src/backend/statistics/mvdistinct.c            -> statext_import.c (free, validate)
    src/backend/statistics/dependencies.c          -> statext_import.c (free, validate)
    src/backend/statistics/mcv.c                   -> statext_import.c (free, import)
    src/backend/statistics/stat_utils.c            -> stat_utils.c (statatt_set_slot,
        statatt_init_empty_tuple, statatt_get_elem_type, statatt_get_range_type,
        statatt_check_bounds_histogram added)

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

Otherwise the bodies are upstream, tabs and all.

### 2.2, extended statistics

The new files are wrapped in `PG_VERSION_NUM >= 160000 && PG_VERSION_NUM <
190000`, and stat_utils.c is widened to the same range, so the objects stay
consistent on 18 (compiles clean there, not run).

All non-static symbols are prefixed `regresql_`. Entry points with
PG_FUNCTION_INFO_V1: regresql_pg_restore_extended_stats,
regresql_pg_clear_extended_stats, regresql_pg_ndistinct_in,
regresql_pg_dependencies_in.

statext_import.c collects the six functions 16/17 lack (statext_ndistinct_free
and _validate, statext_dependencies_free and _validate, statext_mcv_free and
_import) under regresql_ names, with statext_import.h. The serialize and
deserialize routines are the server's own. STATS_MCVLIST_MAX_ITEMS is 10000 on
16/17, the same as MAX_STATISTICS_TARGET.

pg_ndistinct.c and pg_dependencies.c keep only the input parsers, out/recv/send
are dropped, the 16/17 output stays. 16 and 17 differ in
makeJsonLexContextCstringLen (no lex argument and no freeJsonLexContext on 16),
so the pg_*_in functions have a PG_VERSION_NUM >= 170000 branch for that. The
callbacks are upstream, JSON_SEM_ACTION_FAILED exists on 16.

extended_stats_funcs.c: upsert_pg_statistic_ext_data lost its const qualifiers
on the three arrays, and import_mcv, check_mcvlist_array and array_in_safe cast
away const where they call array_contains_nulls, deconstruct_array_builtin and
InputFunctionCallSafe. Those take const pointers only on master. The local
stat_utils.h and statext_import.h replace statistics/stat_utils.h, and
miscnodes.h is included explicitly.

stat_utils.c: besides the statatt_* helpers, stats_fill_fcinfo_from_arg_pairs
accepts a text value for n_distinct and dependencies and parses it with the
same input routines (stats_text_to_bytea), so the argument type check upstream
makes still holds for everything else. A parse error is thrown as a WARNING and
the function returns false. The upstream errcode() additions in that function
were not taken, they are 19-only polish. attribute_stats.c keeps its own static
copies of the REL_18 slot helpers, they are not rewired to the new statatt_*
functions.

Not ported: the 19 output format of pg_ndistinct/pg_dependencies. 16/17 keep
printing `{"1, 2": 11}`; only input understands the JSON array. Unknown literals
(`'...'` without a cast) are not accepted for n_distinct/dependencies, pass
`::pg_ndistinct`, `::pg_dependencies` or `::text`.

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
