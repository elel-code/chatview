# ── Summarise src/chatview sub-libraries ───────────────────────────────────────
add_subdirectory(proto)
add_subdirectory(storage)
add_subdirectory(client)

# ── Main executable ────────────────────────────────────────────────────────────
add_executable(chatview_client ${CMAKE_SOURCE_DIR}/src/main.cpp)
if(WIN32)
    set_target_properties(chatview_client PROPERTIES WIN32_EXECUTABLE TRUE)
    if(MSVC OR CMAKE_CXX_SIMULATE_ID STREQUAL "MSVC")
        target_link_options(chatview_client PRIVATE "LINKER:/ENTRY:mainCRTStartup")
    endif()
endif()

set(CHATVIEW_FRONTEND_DIST_DIR "${CMAKE_SOURCE_DIR}/../frontend/dist" CACHE PATH "Frontend production dist directory to embed")
if(NOT EXISTS "${CHATVIEW_FRONTEND_DIST_DIR}/index.html")
    message(FATAL_ERROR "Frontend dist is required for the production client: ${CHATVIEW_FRONTEND_DIST_DIR}/index.html")
endif()

saucer_embed(
    "${CHATVIEW_FRONTEND_DIST_DIR}"
    DESTINATION "${CMAKE_CURRENT_BINARY_DIR}/embedded"
    TARGET chatview_client
)

target_link_libraries(chatview_client
    PRIVATE
    chatview_client_core
    glaze::glaze
    saucer::saucer
    saucer::embedded
)

# ── Tests ──────────────────────────────────────────────────────────────────────
if(CHATVIEW_BUILD_TESTS)
    enable_testing()

    add_executable(chatview_client_local_tests ${CMAKE_SOURCE_DIR}/tests/client_local_tests.cpp)
    target_link_libraries(chatview_client_local_tests
        PRIVATE
        chatview_client_core
        glaze::glaze
        sqlite_orm
    )
    add_test(NAME chatview_client_local_tests COMMAND chatview_client_local_tests)
endif()
