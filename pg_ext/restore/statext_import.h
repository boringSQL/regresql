/*-------------------------------------------------------------------------
 *
 * statext_import.h
 *	  Declarations for the extended statistics import helpers, named after
 *	  their PostgreSQL 19 counterparts in
 *	  src/include/statistics/extended_stats_internal.h.
 *
 * Local file, see README.md in this directory.
 *
 *-------------------------------------------------------------------------
 */
#ifndef REGRESQL_STATEXT_IMPORT_H
#define REGRESQL_STATEXT_IMPORT_H

#include "statistics/extended_stats_internal.h"

extern bool regresql_statext_ndistinct_validate(const MVNDistinct *ndistinct,
												const int2vector *stxkeys,
												int numexprs, int elevel);
extern void regresql_statext_ndistinct_free(MVNDistinct *ndistinct);

extern bool regresql_statext_dependencies_validate(const MVDependencies *dependencies,
												   const int2vector *stxkeys,
												   int numexprs, int elevel);
extern void regresql_statext_dependencies_free(MVDependencies *dependencies);

extern void regresql_statext_mcv_free(MCVList *mcvlist);
extern Datum regresql_statext_mcv_import(int elevel, int numattrs, Oid *atttypids,
										 int32 *atttypmods, Oid *atttypcolls,
										 int nitems, Datum *mcv_elems,
										 bool *mcv_nulls, float8 *freqs,
										 float8 *base_freqs);

#endif							/* REGRESQL_STATEXT_IMPORT_H */
