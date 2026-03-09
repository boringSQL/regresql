/*
 * pg_regresql.c
 *
 * Force the planner to use the stats from pg_class (relpages, reltupes,
 * etc.) instea of checking the actual file size on disk.
 *
 * The functionality enables full injnection of the production-scale
 * statistics into a small test database.
 *
 * Limitations of default behaviour described in:
 * https://boringsql.com/posts/portable-stats/
 *
 * 
 * Usage: LOAD 'pg_regresql' 
 */

#include "postgres.h"
#include "fmgr.h"

#include "access/htup_details.h"
#include "catalog/pg_class.h"
#include "optimizer/plancat.h"
#include "utils/syscache.h"

PG_MODULE_MAGIC;

PGDLLEXPORT void _PG_init(void);

// hook chaining
static get_relation_info_hook_type prev_hook = NULL;

// regresql hook function
static void
override_relation_stats(PlannerInfo *root,
                        Oid relid,
                        bool inhparent,
                        RelOptInfo *rel)
{
    HeapTuple       tuple;
    Form_pg_class   pg_class_row;
    int32           catalog_pages;
    float4          catalog_tuples;
    int32           catalog_allvisible;

    if (prev_hook != NULL)
        prev_hook(root, relid, inhparent, rel);

    tuple = SearchSysCache1(RELOID, ObjectIdGetDatum(relid));
    if (!HeapTupleIsValid(tuple))
        return;

    // get row data
    pg_class_row = (Form_pg_class) GETSTRUCT(tuple);

    // relpages
    catalog_pages = pg_class_row->relpages;
    // reltuples
    catalog_tuples = pg_class_row->reltuples;
    // all visble pages
    catalog_allvisible = pg_class_row->relallvisible;

    /*
     * Only override if the stats look like they were actually set.
     * relpages == 0 means the table might be empty or never analyzed.
     * reltuples == -1 means "never analyzed" in PostgreSQL.
     */
    // override only if it's set (0 empty; -1 never analyzed)
    if (catalog_pages > 0 && catalog_tuples >= 0)
    {
        rel->pages = catalog_pages;
        rel->tuples = catalog_tuples;

        // fraction
        if (catalog_pages > 0)
            rel->allvisfrac = (double) catalog_allvisible / (double) catalog_pages;
        else
            rel->allvisfrac = 0.0;
    }

    ReleaseSysCache(tuple);

    
    if (rel->indexlist != NIL)
    {
        ListCell *lc;

        foreach(lc, rel->indexlist)
        {
            IndexOptInfo    *idx_info;
            HeapTuple       idx_tuple;
            Form_pg_class   idx_pg_class;
            int32           idx_pages;
            float4          idx_tuples;

            idx_info = (IndexOptInfo *) lfirst(lc);

            idx_tuple = SearchSysCache1(RELOID, ObjectIdGetDatum(idx_info->indexoid));
            if (!HeapTupleIsValid(idx_tuple))
                continue;

            idx_pg_class = (Form_pg_class) GETSTRUCT(idx_tuple);

            idx_pages = idx_pg_class->relpages;
            idx_tuples = idx_pg_class->reltuples;

            if (idx_pages > 0)
                idx_info->pages = idx_pages;

            if (idx_tuples >= 0)
                idx_info->tuples = idx_tuples;

            ReleaseSysCache(idx_tuple);
        }
    }
}

void
_PG_init(void)
{
    // setup the hook function
    prev_hook = get_relation_info_hook;
    get_relation_info_hook = override_relation_stats;
}
