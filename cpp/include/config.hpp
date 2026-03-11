#pragma once

#include <filesystem>
#include <optional>
#include <string>
#include <vector>

namespace ucxsync::config {

struct Credentials {
	std::string username{"Administrator"};
	std::string password{"ultracam"};
};

struct Sync {
	std::string project;
	std::string destination;
	int maxParallelism{8};
	int serviceLoopIntervalSeconds{10};
	long long minFreeDiskSpace{52'428'800};
	long long diskSpaceSafetyMargin{104'857'600};
};

struct Web {
	std::string host{"localhost"};
	int port{8080};
};

struct Monitoring {
	int performanceUpdateIntervalSeconds{1};
	int uiUpdateIntervalSeconds{2};
	int cpuSmoothingSamples{3};
	double maxDiskThroughputMbps{200.0};
	long long networkSpeedBps{1'000'000'000};
};

struct Logging {
	std::string level{"info"};
	std::string file{"logs/ucxsync.log"};
	int maxSize{100};
	int maxBackups{5};
	int maxAge{30};
};

struct Config {
	std::vector<std::string> nodes;
	std::vector<std::string> shares;
	Credentials credentials;
	Sync sync;
	Web web;
	Monitoring monitoring;
	Logging logging;
};

Config defaults();
std::optional<std::filesystem::path> resolveConfigPath(const std::optional<std::filesystem::path>& explicitPath);
bool load(const std::optional<std::filesystem::path>& explicitPath, Config& outConfig, std::string& errorMessage);
std::string validate(const Config& config);

} // namespace ucxsync::config
