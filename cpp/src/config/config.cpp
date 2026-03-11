#include "config.hpp"

#include <cstdlib>
#include <yaml-cpp/yaml.h>

namespace ucxsync::config {

namespace {

constexpr const char* kDefaultNodes[] = {
	"WU01", "WU02", "WU03", "WU04", "WU05", "WU06", "WU07",
	"WU08", "WU09", "WU10", "WU11", "WU12", "WU13", "CU"
};

int parseDurationSeconds(const YAML::Node& node, int fallback) {
	if (!node || !node.IsScalar()) {
		return fallback;
	}

	const auto value = node.as<std::string>();
	if (value.empty()) {
		return fallback;
	}

	try {
		if (value.back() == 's') {
			return std::stoi(value.substr(0, value.size() - 1));
		}
		if (value.back() == 'm') {
			return std::stoi(value.substr(0, value.size() - 1)) * 60;
		}
		return std::stoi(value);
	} catch (...) {
		return fallback;
	}
}

void applySequence(const YAML::Node& node, std::vector<std::string>& out) {
	if (!node || !node.IsSequence()) {
		return;
	}

	out.clear();
	for (const auto& item : node) {
		out.push_back(item.as<std::string>());
	}
}

void applyScalar(const YAML::Node& parent, const char* key, std::string& target) {
	if (parent && parent[key]) {
		target = parent[key].as<std::string>();
	}
}

void applyScalar(const YAML::Node& parent, const char* key, int& target) {
	if (parent && parent[key]) {
		target = parent[key].as<int>();
	}
}

void applyScalar(const YAML::Node& parent, const char* key, long long& target) {
	if (parent && parent[key]) {
		target = parent[key].as<long long>();
	}
}

void applyScalar(const YAML::Node& parent, const char* key, double& target) {
	if (parent && parent[key]) {
		target = parent[key].as<double>();
	}
}

void applyEnvInt(const char* key, int& target) {
	if (const char* value = std::getenv(key)) {
		try {
			target = std::stoi(value);
		} catch (...) {
		}
	}
}

void applyEnvString(const char* key, std::string& target) {
	if (const char* value = std::getenv(key)) {
		target = value;
	}
}

} // namespace

Config defaults() {
	Config cfg;
	cfg.nodes.assign(std::begin(kDefaultNodes), std::end(kDefaultNodes));
	cfg.shares = {"E$", "F$"};
	return cfg;
}

std::optional<std::filesystem::path> resolveConfigPath(const std::optional<std::filesystem::path>& explicitPath) {
	if (explicitPath && !explicitPath->empty()) {
		if (std::filesystem::exists(*explicitPath)) {
			return explicitPath;
		}
		return std::nullopt;
	}

	const std::filesystem::path local{"config.yaml"};
	if (std::filesystem::exists(local)) {
		return local;
	}

	if (const char* home = std::getenv("HOME")) {
		const auto homeConfig = std::filesystem::path(home) / ".ucxsync" / "config.yaml";
		if (std::filesystem::exists(homeConfig)) {
			return homeConfig;
		}
	}

	const std::filesystem::path systemConfig{"/etc/ucxsync/config.yaml"};
	if (std::filesystem::exists(systemConfig)) {
		return systemConfig;
	}

	return std::nullopt;
}

bool load(const std::optional<std::filesystem::path>& explicitPath, Config& outConfig, std::string& errorMessage) {
	outConfig = defaults();

	const auto path = resolveConfigPath(explicitPath);
	if (explicitPath && !explicitPath->empty() && !path) {
		errorMessage = "configuration file not found: " + explicitPath->string();
		return false;
	}

	if (path) {
		try {
			const auto root = YAML::LoadFile(path->string());

			applySequence(root["nodes"], outConfig.nodes);
			applySequence(root["shares"], outConfig.shares);

			const auto credentials = root["credentials"];
			applyScalar(credentials, "username", outConfig.credentials.username);
			applyScalar(credentials, "password", outConfig.credentials.password);

			const auto sync = root["sync"];
			applyScalar(sync, "project", outConfig.sync.project);
			applyScalar(sync, "destination", outConfig.sync.destination);
			applyScalar(sync, "max_parallelism", outConfig.sync.maxParallelism);
			applyScalar(sync, "min_free_disk_space", outConfig.sync.minFreeDiskSpace);
			applyScalar(sync, "disk_space_safety_margin", outConfig.sync.diskSpaceSafetyMargin);
			if (sync && sync["service_loop_interval"]) {
				outConfig.sync.serviceLoopIntervalSeconds = parseDurationSeconds(sync["service_loop_interval"], outConfig.sync.serviceLoopIntervalSeconds);
			}

			const auto web = root["web"];
			applyScalar(web, "host", outConfig.web.host);
			applyScalar(web, "port", outConfig.web.port);

			const auto monitoring = root["monitoring"];
			if (monitoring && monitoring["performance_update_interval"]) {
				outConfig.monitoring.performanceUpdateIntervalSeconds = parseDurationSeconds(monitoring["performance_update_interval"], outConfig.monitoring.performanceUpdateIntervalSeconds);
			}
			if (monitoring && monitoring["ui_update_interval"]) {
				outConfig.monitoring.uiUpdateIntervalSeconds = parseDurationSeconds(monitoring["ui_update_interval"], outConfig.monitoring.uiUpdateIntervalSeconds);
			}
			applyScalar(monitoring, "cpu_smoothing_samples", outConfig.monitoring.cpuSmoothingSamples);
			applyScalar(monitoring, "max_disk_throughput_mbps", outConfig.monitoring.maxDiskThroughputMbps);
			applyScalar(monitoring, "network_speed_bps", outConfig.monitoring.networkSpeedBps);

			const auto logging = root["logging"];
			applyScalar(logging, "level", outConfig.logging.level);
			applyScalar(logging, "file", outConfig.logging.file);
			applyScalar(logging, "max_size", outConfig.logging.maxSize);
			applyScalar(logging, "max_backups", outConfig.logging.maxBackups);
			applyScalar(logging, "max_age", outConfig.logging.maxAge);
		} catch (const std::exception& ex) {
			errorMessage = std::string("failed to load YAML: ") + ex.what();
			return false;
		}
	}

	applyEnvString("UCXSYNC_WEB_HOST", outConfig.web.host);
	applyEnvInt("UCXSYNC_WEB_PORT", outConfig.web.port);
	applyEnvInt("UCXSYNC_SYNC_MAX_PARALLELISM", outConfig.sync.maxParallelism);

	errorMessage = validate(outConfig);
	return errorMessage.empty();
}

std::string validate(const Config& config) {
	if (config.nodes.empty()) {
		return "no nodes configured";
	}
	if (config.shares.empty()) {
		return "no shares configured";
	}
	if (config.sync.maxParallelism < 1) {
		return "sync.max_parallelism must be at least 1";
	}
	if (config.web.port < 1 || config.web.port > 65535) {
		return "web.port must be in range 1..65535";
	}
	return {};
}

} // namespace ucxsync::config
