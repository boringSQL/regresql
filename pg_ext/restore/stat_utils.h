/*-------------------------------------------------------------------------
 *
 * stat_utils.h
 *	  Extended statistics and selectivity estimation functions.
 *
 * Portions Copyright (c) 1996-2025, PostgreSQL Global Development Group
 * Portions Copyright (c) 1994, Regents of the University of California
 *
 * src/include/statistics/stat_utils.h
 *
 * Ported from PostgreSQL REL_18_STABLE at commit
 * 780900dcfab (2026-11-09).
 * Source: src/include/statistics/stat_utils.h
 *
 * Local modifications are listed in README.md in this directory.
 *
 *-------------------------------------------------------------------------
 */
#ifndef STATS_UTILS_H
#define STATS_UTILS_H

#include "access/attnum.h"
#include "fmgr.h"

/* avoid including primnodes.h here */
struct RangeVar;

/* avoid including typcache.h here */
typedef struct TypeCacheEntry TypeCacheEntry;

struct StatsArgInfo
{
	const char *argname;
	Oid			argtype;
};

extern void regresql_stats_check_required_arg(FunctionCallInfo fcinfo,
											  struct StatsArgInfo *arginfo,
											  int argnum);
extern bool regresql_stats_check_arg_array(FunctionCallInfo fcinfo,
										   struct StatsArgInfo *arginfo, int argnum);
extern bool regresql_stats_check_arg_pair(FunctionCallInfo fcinfo,
										  struct StatsArgInfo *arginfo,
										  int argnum1, int argnum2);

extern void regresql_RangeVarCallbackForStats(const struct RangeVar *relation,
											  Oid relId, Oid oldRelid, void *arg);

extern bool regresql_stats_fill_fcinfo_from_arg_pairs(FunctionCallInfo pairs_fcinfo,
													  FunctionCallInfo positional_fcinfo,
													  struct StatsArgInfo *arginfo);

extern void regresql_statatt_init_empty_tuple(Oid reloid, int16 attnum, bool inherited,
											  Datum *values, bool *nulls, bool *replaces);

extern void regresql_statatt_set_slot(Datum *values, bool *nulls, bool *replaces,
									  int16 stakind, Oid staop, Oid stacoll,
									  Datum stanumbers, bool stanumbers_isnull,
									  Datum stavalues, bool stavalues_isnull);

extern bool regresql_statatt_get_elem_type(TypeCacheEntry *basetypcache,
										   Oid *elemtypid, Oid *elem_eq_opr);
extern bool regresql_statatt_get_range_type(TypeCacheEntry *basetypcache,
											Oid *rangetypid);


/* input routines of the 2.2 pg_ndistinct / pg_dependencies parsers */
extern Datum regresql_pg_ndistinct_in(PG_FUNCTION_ARGS);
extern Datum regresql_pg_dependencies_in(PG_FUNCTION_ARGS);

#endif							/* STATS_UTILS_H */
