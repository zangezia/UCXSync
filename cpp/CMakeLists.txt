cmake_minimum_required(VERSION 3.16)

project(ucxsync_cpp LANGUAGES CXX)

if(WIN32)
	message(FATAL_ERROR "The experimental C++ port is Linux-only.")
endif()

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

include(FetchContent)

FetchContent_Declare(
	yaml-cpp
	GIT_REPOSITORY https://github.com/jbeder/yaml-cpp.git
	GIT_TAG yaml-cpp-0.8.0
)

FetchContent_MakeAvailable(yaml-cpp)

add_library(ucxsync_config STATIC
	src/config/config.cpp
)

target_include_directories(ucxsync_config
	PUBLIC ${CMAKE_CURRENT_SOURCE_DIR}/include
)

target_link_libraries(ucxsync_config
	PUBLIC yaml-cpp::yaml-cpp
)

add_executable(ucxsync_cpp
	src/main.cpp
)

target_link_libraries(ucxsync_cpp
	PRIVATE ucxsync_config
)
