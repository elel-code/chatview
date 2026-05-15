add_library(sqlite3 STATIC)
target_sources(sqlite3
    PRIVATE
        ${CMAKE_CURRENT_SOURCE_DIR}/sqlite3.c
)
target_include_directories(sqlite3
    PUBLIC
        ${CMAKE_CURRENT_SOURCE_DIR}
)
