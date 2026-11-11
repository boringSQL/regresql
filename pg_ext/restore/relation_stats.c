/*-------------------------------------------------------------------------
 * relation_stats.c
 *
 *	  PostgreSQL relation statistics manipulation
 *
 * Code supporting the direct import of relation statistics, similar to
 * what is done by the ANALYZE command.
 *
 * Portions Copyright (c) 1996-2025, PostgreSQL Global Development Group
 * Portions Copyright (c) 1994, Regents of the University of California
 *
 * IDENTIFICATION
 *       src/backend/statistics/relation_stats.c
 *
 * Ported from PostgreSQL REL_18_STABLE at commit
 * 780900dcfab (2026-11-09).
 * Source: src/backend/statistics/relation_stats.c
 *
 * Local modifications are listed in README.md in this directory.
 *
 *-------------------------------------------------------------------------
 */

#include "postgres.h"

#if PG_VERSION_NUM >= 160000 && PG_VERSION_NUM < 180000

#include <math.h>

#include "access/heapam.h"
#include "catalog/indexing.h"
#include "catalog/namespace.h"
#include "nodes/makefuncs.h"
#include "stat_utils.h"
#include "utils/builtins.h"
#include "utils/fmgroids.h"
#include "utils/fmgrprotos.h"
#include "utils/lsyscache.h"
#include "utils/syscache.h"


/*
 * Positional argument numbers, names, and types for
 * relation_statistics_update().
 */

enum relation_stats_argnum
{
	RELSCHEMA_ARG = 0,
	RELNAME_ARG,
	RELPAGES_ARG,
	RELTUPLES_ARG,
	RELALLVISIBLE_ARG,
	RELALLFROZEN_ARG,
	NUM_RELATION_STATS_ARGS
};

static struct StatsArgInfo relarginfo[] =
{
	[RELSCHEMA_ARG] = {"schemaname", TEXTOID},
	[RELNAME_ARG] = {"relname", TEXTOID},
	[RELPAGES_ARG] = {"relpages", INT4OID},
	[RELTUPLES_ARG] = {"reltuples", FLOAT4OID},
	[RELALLVISIBLE_ARG] = {"relallvisible", INT4OID},
	[RELALLFROZEN_ARG] = {"relallfrozen", INT4OID},
	[NUM_RELATION_STATS_ARGS] = {0}
};

static bool relation_statistics_update(FunctionCallInfo fcinfo);

/*
 * Internal function for modifying statistics for a relation.
 */
static bool
relation_statistics_update(FunctionCallInfo fcinfo)
{
	bool		result = true;
	char	   *nspname;
	char	   *relname;
	Oid			reloid;
	Relation	crel;
	BlockNumber relpages = 0;
	bool		update_relpages = false;
	float		reltuples = 0;
	bool		update_reltuples = false;
	BlockNumber relallvisible = 0;
	bool		update_relallvisible = false;
	HeapTuple	ctup;
	Form_pg_class pgcform;
	int			replaces[3] = {0};
	Datum		values[3] = {0};
	bool		nulls[3] = {0};
	int			nreplaces = 0;
	Oid			locked_table = InvalidOid;

	regresql_stats_check_required_arg(fcinfo, relarginfo, RELSCHEMA_ARG);
	regresql_stats_check_required_arg(fcinfo, relarginfo, RELNAME_ARG);

	nspname = TextDatumGetCString(PG_GETARG_DATUM(RELSCHEMA_ARG));
	relname = TextDatumGetCString(PG_GETARG_DATUM(RELNAME_ARG));

	if (RecoveryInProgress())
		ereport(ERROR,
				(errcode(ERRCODE_OBJECT_NOT_IN_PREREQUISITE_STATE),
				 errmsg("recovery is in progress"),
				 errhint("Statistics cannot be modified during recovery.")));

	reloid = RangeVarGetRelidExtended(makeRangeVar(nspname, relname, -1),
									  ShareUpdateExclusiveLock, 0,
									  regresql_RangeVarCallbackForStats, &locked_table);

	if (!PG_ARGISNULL(RELPAGES_ARG))
	{
		relpages = PG_GETARG_UINT32(RELPAGES_ARG);
		update_relpages = true;
	}

	if (!PG_ARGISNULL(RELTUPLES_ARG))
	{
		reltuples = PG_GETARG_FLOAT4(RELTUPLES_ARG);
		if (isnan(reltuples) || isinf(reltuples))
		{
			ereport(WARNING,
					(errcode(ERRCODE_INVALID_PARAMETER_VALUE),
					 errmsg("argument \"%s\" must be a finite value", "reltuples")));
			result = false;
		}
		else if (reltuples < -1.0)
		{
			ereport(WARNING,
					(errcode(ERRCODE_INVALID_PARAMETER_VALUE),
					 errmsg("argument \"%s\" must not be less than -1.0", "reltuples")));
			result = false;
		}
		else
			update_reltuples = true;
	}

	if (!PG_ARGISNULL(RELALLVISIBLE_ARG))
	{
		relallvisible = PG_GETARG_UINT32(RELALLVISIBLE_ARG);
		update_relallvisible = true;
	}

	/*
	 * Take RowExclusiveLock on pg_class, consistent with
	 * vac_update_relstats().
	 */
	crel = table_open(RelationRelationId, RowExclusiveLock);

	ctup = SearchSysCache1(RELOID, ObjectIdGetDatum(reloid));
	if (!HeapTupleIsValid(ctup))
		elog(ERROR, "pg_class entry for relid %u not found", reloid);

	pgcform = (Form_pg_class) GETSTRUCT(ctup);

	if (update_relpages && relpages != pgcform->relpages)
	{
		replaces[nreplaces] = Anum_pg_class_relpages;
		values[nreplaces] = UInt32GetDatum(relpages);
		nreplaces++;
	}

	if (update_reltuples && reltuples != pgcform->reltuples)
	{
		replaces[nreplaces] = Anum_pg_class_reltuples;
		values[nreplaces] = Float4GetDatum(reltuples);
		nreplaces++;
	}

	if (update_relallvisible && relallvisible != pgcform->relallvisible)
	{
		replaces[nreplaces] = Anum_pg_class_relallvisible;
		values[nreplaces] = UInt32GetDatum(relallvisible);
		nreplaces++;
	}

	if (nreplaces > 0)
	{
		TupleDesc	tupdesc = RelationGetDescr(crel);
		HeapTuple	newtup;

		newtup = heap_modify_tuple_by_cols(ctup, tupdesc, nreplaces,
										   replaces, values, nulls);
		CatalogTupleUpdate(crel, &newtup->t_self, newtup);
		heap_freetuple(newtup);
	}

	ReleaseSysCache(ctup);

	/* release the lock, consistent with vac_update_relstats() */
	table_close(crel, RowExclusiveLock);

	CommandCounterIncrement();

	return result;
}

PG_FUNCTION_INFO_V1(regresql_pg_restore_relation_stats);

Datum
regresql_pg_restore_relation_stats(PG_FUNCTION_ARGS)
{
	LOCAL_FCINFO(positional_fcinfo, NUM_RELATION_STATS_ARGS);
	bool		result = true;

	InitFunctionCallInfoData(*positional_fcinfo, NULL,
							 NUM_RELATION_STATS_ARGS,
							 InvalidOid, NULL, NULL);

	if (!regresql_stats_fill_fcinfo_from_arg_pairs(fcinfo, positional_fcinfo,
												   relarginfo))
		result = false;

	if (!relation_statistics_update(positional_fcinfo))
		result = false;

	PG_RETURN_BOOL(result);
}

#endif							/* PG_VERSION_NUM >= 160000 && PG_VERSION_NUM < 180000 */
