# ── Cache database (SQLite-based) ──────────────────────────────────────────────
add_library(chatview_storage_cache STATIC)
target_sources(chatview_storage_cache
    PUBLIC
    FILE_SET CXX_MODULES
    FILES
    cache_database.cppm
)
target_link_libraries(chatview_storage_cache PUBLIC sqlite_orm)
