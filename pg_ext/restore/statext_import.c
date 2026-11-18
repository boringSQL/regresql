/*-------------------------------------------------------------------------
 *
 * statext_import.c
 *	  Validation and import helpers for extended statistics objects.
 *
 * Portions Copyright (c) 1996-2026, PostgreSQL Global Development Group
 * Portions Copyright (c) 1994, Regents of the University of California
 *
 * IDENTIFICATION
 *	  src/backend/statistics/mvdistinct.c
 *	  src/backend/statistics/dependencies.c
 *	  src/backend/statistics/mcv.c
 *
 * Ported from PostgreSQL master at commit 061065e28f0 (2026-10-08).
 * Source: the functions statext_ndistinct_free, statext_ndistinct_validate,
 * statext_dependencies_free, statext_dependencies_validate, statext_mcv_free
 * and statext_mcv_import of the files above, which 16 and 17 do not have.
 *
 * Local modifications are listed in README.md in this directory.
 *
 *-------------------------------------------------------------------------
 */

#include "postgres.h"

#if PG_VERSION_NUM >= 160000 && PG_VERSION_NUM < 190000

#include "access/htup_details.h"
#include "catalog/pg_type.h"
#include "commands/vacuum.h"
#include "fmgr.h"
#include "nodes/miscnodes.h"
#include "statistics/extended_stats_internal.h"
#include "statext_import.h"
#include "utils/builtins.h"
#include "utils/lsyscache.h"
#include "utils/syscache.h"


/*
 * Free allocations of a MVNDistinct.
 */
void
regresql_statext_ndistinct_free(MVNDistinct *ndistinct)
{
	for (uint32 i = 0; i < ndistinct->nitems; i++)
		pfree(ndistinct->items[i].attributes);
	pfree(ndistinct);
}

/*
 * Validate a set of MVNDistincts against the extended statistics object
 * definition.
 *
 * Every MVNDistinctItem must be checked to ensure that the attnums in the
 * attributes list correspond to attnums/expressions defined by the extended
 * statistics object.
 *
 * Positive attnums are attributes which must be found in the stxkeys,
 * while negative attnums correspond to an expression number, no attribute
 * number can be below (0 - numexprs).
 */
bool
regresql_statext_ndistinct_validate(const MVNDistinct *ndistinct,
						   const int2vector *stxkeys,
						   int numexprs, int elevel)
{
	int			attnum_expr_lowbound = 0 - numexprs;

	/* Scan through each MVNDistinct entry */
	for (uint32 i = 0; i < ndistinct->nitems; i++)
	{
		MVNDistinctItem item = ndistinct->items[i];

		/*
		 * Cross-check each attribute in a MVNDistinct entry with the extended
		 * stats object definition.
		 */
		for (int j = 0; j < item.nattributes; j++)
		{
			AttrNumber	attnum = item.attributes[j];
			bool		ok = false;

			if (attnum > 0)
			{
				/* attribute number in stxkeys */
				for (int k = 0; k < stxkeys->dim1; k++)
				{
					if (attnum == stxkeys->values[k])
					{
						ok = true;
						break;
					}
				}
			}
			else if ((attnum < 0) && (attnum >= attnum_expr_lowbound))
			{
				/* attribute number for an expression */
				ok = true;
			}

			if (!ok)
			{
				ereport(elevel,
						(errcode(ERRCODE_INVALID_TEXT_REPRESENTATION),
						 errmsg("could not validate \"%s\" object: invalid attribute number %d found",
								"pg_ndistinct", attnum)));
				return false;
			}
		}
	}

	return true;
}

/*
 * Free allocations of a MVDependencies.
 */
void
regresql_statext_dependencies_free(MVDependencies *dependencies)
{
	for (uint32 i = 0; i < dependencies->ndeps; i++)
		pfree(dependencies->deps[i]);
	pfree(dependencies);
}

/*
 * Validate a set of MVDependencies against the extended statistics object
 * definition.
 *
 * Every MVDependencies must be checked to ensure that the attnums in the
 * attributes list correspond to attnums/expressions defined by the
 * extended statistics object.
 *
 * Positive attnums are attributes which must be found in the stxkeys, while
 * negative attnums correspond to an expression number, no attribute number
 * can be below (0 - numexprs).
 */
bool
regresql_statext_dependencies_validate(const MVDependencies *dependencies,
							  const int2vector *stxkeys,
							  int numexprs, int elevel)
{
	int			attnum_expr_lowbound = 0 - numexprs;

	/* Scan through each dependency entry */
	for (uint32 i = 0; i < dependencies->ndeps; i++)
	{
		const MVDependency *dep = dependencies->deps[i];

		/*
		 * Cross-check each attribute in a dependency entry with the extended
		 * stats object definition.
		 */
		for (int j = 0; j < dep->nattributes; j++)
		{
			AttrNumber	attnum = dep->attributes[j];
			bool		ok = false;

			if (attnum > 0)
			{
				/* attribute number in stxkeys */
				for (int k = 0; k < stxkeys->dim1; k++)
				{
					if (attnum == stxkeys->values[k])
					{
						ok = true;
						break;
					}
				}
			}
			else if ((attnum < 0) && (attnum >= attnum_expr_lowbound))
			{
				/* attribute number for an expression */
				ok = true;
			}

			if (!ok)
			{
				ereport(elevel,
						(errcode(ERRCODE_INVALID_TEXT_REPRESENTATION),
						 errmsg("could not validate \"%s\" object: invalid attribute number %d found",
								"pg_dependencies", attnum)));
				return false;
			}
		}
	}

	return true;
}

/*
 * Free allocations of a MCVList.
 */
void
regresql_statext_mcv_free(MCVList *mcvlist)
{
	for (uint32 i = 0; i < mcvlist->nitems; i++)
	{
		MCVItem    *item = &mcvlist->items[i];

		pfree(item->values);
		pfree(item->isnull);
	}
	pfree(mcvlist);
}

/*
 * Create the MCV composite datum, which is a serialization of an array of
 * MCVItems.
 *
 * The inputs consist of four separate arrays of equal length "numitems"
 * (mcv_elems, mcv_nulls, freqs and base_freqs) that form the basics of
 * what is stored in the catalogs.  These form an array of composite
 * records defined by the three atttypX arrays of equal length "numattrs".
 *
 * If any data element fails to convert to the input type specified for that
 * attribute, then function will return a NULL Datum if elevel < ERROR.
 */
Datum
regresql_statext_mcv_import(int elevel, int numattrs,
				   Oid *atttypids, int32 *atttypmods, Oid *atttypcolls,
				   int nitems, Datum *mcv_elems, bool *mcv_nulls,
				   float8 *freqs, float8 *base_freqs)
{
	MCVList    *mcvlist;
	bytea	   *bytes;
	VacAttrStats **vastats;

	/*
	 * Allocate the MCV list structure, set the global parameters.
	 */
	mcvlist = (MCVList *) palloc0(offsetof(MCVList, items) +
								  (sizeof(MCVItem) * nitems));

	mcvlist->magic = STATS_MCV_MAGIC;
	mcvlist->type = STATS_MCV_TYPE_BASIC;
	mcvlist->ndimensions = numattrs;
	mcvlist->nitems = nitems;

	/* Set the values for the 1-D arrays and allocate space for the 2-D arrays */
	for (int i = 0; i < nitems; i++)
	{
		MCVItem    *item = &mcvlist->items[i];

		item->frequency = freqs[i];
		item->base_frequency = base_freqs[i];
		item->values = (Datum *) palloc0_array(Datum, numattrs);
		item->isnull = (bool *) palloc0_array(bool, numattrs);
	}

	/*
	 * Walk through each dimension, determine the input function for that
	 * type, and then attempt to convert all values in that column via that
	 * function.  We approach this column-wise because it is simpler to deal
	 * with one input function at time, and possibly more cache-friendly.
	 */
	for (int j = 0; j < numattrs; j++)
	{
		FmgrInfo	finfo;
		Oid			ioparam;
		Oid			infunc;
		int			index = j;

		getTypeInputInfo(atttypids[j], &infunc, &ioparam);
		fmgr_info(infunc, &finfo);

		/* store info about data type OIDs */
		mcvlist->types[j] = atttypids[j];

		for (int i = 0; i < nitems; i++)
		{
			MCVItem    *item = &mcvlist->items[i];

			if (mcv_nulls[index])
			{
				/* NULL value detected, hence no input to process */
				item->values[j] = (Datum) 0;
				item->isnull[j] = true;
			}
			else
			{
				char	   *s = TextDatumGetCString(mcv_elems[index]);
				ErrorSaveContext escontext = {T_ErrorSaveContext};

				if (!InputFunctionCallSafe(&finfo, s, ioparam, atttypmods[j],
										   (Node *) &escontext, &item->values[j]))
				{
					ereport(elevel,
							(errcode(ERRCODE_INVALID_PARAMETER_VALUE),
							 errmsg("could not parse MCV element \"%s\": incorrect value", s)));
					pfree(s);
					goto error;
				}

				pfree(s);
			}

			index += numattrs;
		}
	}

	/*
	 * The function statext_mcv_serialize() requires an array of pointers to
	 * VacAttrStats records, but only a few fields within those records have
	 * to be filled out.
	 */
	vastats = (VacAttrStats **) palloc0_array(VacAttrStats *, numattrs);

	for (int i = 0; i < numattrs; i++)
	{
		Oid			typid = atttypids[i];
		HeapTuple	typtuple;

		typtuple = SearchSysCacheCopy1(TYPEOID, ObjectIdGetDatum(typid));

		if (!HeapTupleIsValid(typtuple))
			elog(ERROR, "cache lookup failed for type %u", typid);

		vastats[i] = palloc0_object(VacAttrStats);

		vastats[i]->attrtype = (Form_pg_type) GETSTRUCT(typtuple);
		vastats[i]->attrtypid = typid;
		vastats[i]->attrcollid = atttypcolls[i];
	}

	bytes = statext_mcv_serialize(mcvlist, vastats);

	for (int i = 0; i < numattrs; i++)
	{
		pfree(vastats[i]);
	}
	pfree((void *) vastats);

	pfree(mcv_elems);
	pfree(mcv_nulls);

	if (bytes == NULL)
	{
		ereport(elevel,
				(errcode(ERRCODE_INVALID_PARAMETER_VALUE),
				 errmsg("could not import MCV list")));
		goto error;
	}

	return PointerGetDatum(bytes);

error:
	regresql_statext_mcv_free(mcvlist);
	return (Datum) 0;
}

#endif							/* PG_VERSION_NUM >= 160000 && PG_VERSION_NUM < 190000 */
