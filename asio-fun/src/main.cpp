#include <iostream>
#include <asio.hpp>
#include <string>

using asio::ip::tcp;

asio::awaitable<void> handle_client(tcp::socket socket) {
    try {
        char buffer[1024];
        
        std::cout << "[Server] New client connected!" << std::endl;
        
        while (true) {
            // Read data from the client
            std::size_t n = co_await socket.async_read_some(
                asio::buffer(buffer), 
                asio::use_awaitable
            );
            
            std::string received_data(buffer, n);
            std::cout << "[Server] Received: " << received_data << std::endl;
            
            // Optional: Echo back to client
            std::string response = "Echo: " + received_data;
            co_await asio::async_write(socket, asio::buffer(response), asio::use_awaitable);
        }
        
    } catch (const std::exception& e) {
        std::cout << "[Server] Client disconnected: " << e.what() << std::endl;
    }
}

asio::awaitable<void> server() {
    auto executor = co_await asio::this_coro::executor;
    tcp::acceptor acceptor(executor, tcp::endpoint(tcp::v4(), 8080));
    
    std::cout << "[Server] Listening on port 8080..." << std::endl;
    std::cout << "[Server] Connect with: nc localhost 8080" << std::endl;
    
    while (true) {
        tcp::socket socket = co_await acceptor.async_accept(asio::use_awaitable);
        
        //new coro for each client
        asio::co_spawn(executor, handle_client(std::move(socket)), asio::detached);
    }
}

asio::awaitable<void> dooFoo(int t) {
    asio::steady_timer timer(co_await asio::this_coro::executor, std::chrono::seconds(t));
    std::cout << "Starting " << t << " second wait..." << std::endl;
    co_await timer.async_wait(asio::use_awaitable);

    std::cout << t << " second wait complete!" << std::endl; 
}

int main() {
    asio::io_context io;
    asio::co_spawn(io, dooFoo(10), asio::detached);
    asio::co_spawn(io, dooFoo(3), asio::detached);
    asio::co_spawn(io, server(), asio::detached);
    io.run();
    return 0;
}