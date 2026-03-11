#include "config.hpp"

#include <iostream>
#include <optional>

int main(int argc, char** argv) {
	std::optional<std::filesystem::path> configPath;
	if (argc > 1) {
		configPath = std::filesystem::path(argv[1]);
	}

	ucxsync::config::Config config;
	std::string error;
	if (!ucxsync::config::load(configPath, config, error)) {
		std::cerr << "Config load failed: " << error << '\n';
		return 1;
	}

	std::cout << "UCXSync C++ config prototype\n";
	std::cout << "Nodes: " << config.nodes.size() << '\n';
	std::cout << "Shares: " << config.shares.size() << '\n';
	std::cout << "Web: " << config.web.host << ':' << config.web.port << '\n';
	std::cout << "Parallelism: " << config.sync.maxParallelism << '\n';

	return 0;
}
