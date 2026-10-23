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
 * 1370a7832a2ab7bda5625fd1f0448808b534cd50 (2026-10-07).
 * Source: src/include/statistics/stat_utils.h
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

extern void stats_check_required_arg(FunctionCallInfo fcinfo,
									 struct StatsArgInfo *arginfo,
									 int argnum);
extern bool stats_check_arg_array(FunctionCallInfo fcinfo,
								  struct StatsArgInfo *arginfo, int argnum);
extern bool stats_check_arg_pair(FunctionCallInfo fcinfo,
								 struct StatsArgInfo *arginfo,
								 int argnum1, int argnum2);

extern void RangeVarCallbackForStats(const struct RangeVar *relation,
									 Oid relId, Oid oldRelid, void *arg);

extern bool stats_fill_fcinfo_from_arg_pairs(FunctionCallInfo pairs_fcinfo,
											 FunctionCallInfo positional_fcinfo,
											 struct StatsArgInfo *arginfo);

#endif							/* STATS_UTILS_H */
