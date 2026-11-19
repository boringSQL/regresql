.DEFAULT_GOAL := all

MODULE_big = pg_regresql
OBJS = pg_regresql.o restore/relation_stats.o restore/attribute_stats.o restore/stat_utils.o \
	restore/extended_stats_funcs.o restore/statext_import.o restore/pg_ndistinct.o restore/pg_dependencies.o
EXTENSION = pg_regresql
DATA = pg_regresql--2.0.sql pg_regresql--2.1.sql pg_regresql--2.2.sql pg_regresql--2.1--2.2.sql
EXT_VERSION = 2.2.0
DISTNAME = $(EXTENSION)-$(EXT_VERSION)
DIST_FILES = META.json Makefile $(EXTENSION).control $(DATA) $(EXTENSION).c README.md LICENSE
RESTORE_FILES = restore/relation_stats.c restore/attribute_stats.c restore/stat_utils.c restore/stat_utils.h \
	restore/extended_stats_funcs.c restore/statext_import.c restore/statext_import.h restore/pg_ndistinct.c restore/pg_dependencies.c restore/statistics_format.h restore/README.md

dist:
	rm -rf /tmp/$(DISTNAME) $(DISTNAME).zip
	mkdir -p /tmp/$(DISTNAME)/restore
	cp $(DIST_FILES) /tmp/$(DISTNAME)/
	cp $(RESTORE_FILES) /tmp/$(DISTNAME)/restore/
	cd /tmp && zip -r $(CURDIR)/$(DISTNAME).zip $(DISTNAME)/
	rm -rf /tmp/$(DISTNAME)

clean:
	rm -f $(OBJS) $(OBJS:.o=.bc) $(MODULE_big).so $(MODULE_big).dylib $(MODULE_big).bc
	rm -rf .deps

ifeq ($(filter clean dist,$(MAKECMDGOALS)),)
ifndef PG_SOURCE
PG_CONFIG ?= pg_config
PGXS := $(shell $(PG_CONFIG) --pgxs)
include $(PGXS)
else
subdir = contrib/pg_regresql
top_builddir = $(PG_SOURCE)
top_srcdir = $(PG_SOURCE)
include $(PG_SOURCE)/src/Makefile.global
include $(PG_SOURCE)/contrib/contrib-global.mk
endif
endif
