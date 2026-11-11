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

#include "fmgr.h"

/* avoid including primnodes.h here */
struct RangeVar;

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

#endif							/* STATS_UTILS_H */
