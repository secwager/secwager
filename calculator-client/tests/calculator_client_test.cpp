#include <gtest/gtest.h>
#include "calculator_client.h"
#include "mock_http_client.h"
#include <memory>

class CalculatorClientTest : public ::testing::Test
{
protected:
    std::unique_ptr<MockHttpClient> mock_client;

    void SetUp() override
    {
        mock_client = std::make_unique<MockHttpClient>();
    }
};

TEST_F(CalculatorClientTest, MultiplySuccessful)
{
    // Arrange
    mock_client->QueueResponse(HttpResponse{200, R"({"result": 6})"});
    CalculatorClient client(std::move(mock_client));

    // Act
    auto result = client.multiply(2, 3);

    // Assert
    ASSERT_TRUE(result.has_value());
    EXPECT_EQ(result.value(), 6);
}

TEST_F(CalculatorClientTest, MultiplyUsesCorrectPath)
{
    // Arrange
    auto *mock_ptr = mock_client.get(); // Keep pointer before moving
    mock_client->QueueResponse(HttpResponse{200, R"({"result": 10})"});
    CalculatorClient client(std::move(mock_client));

    // Act
    client.multiply(2, 5);

    // Assert
    EXPECT_EQ(mock_ptr->GetLastPath(), "/multiply/2/5");
}

TEST_F(CalculatorClientTest, MultiplyNetworkError)
{
    // Arrange
    mock_client->QueueResponse(std::nullopt); // Network failure
    CalculatorClient client(std::move(mock_client));

    // Act
    auto result = client.multiply(2, 3);

    // Assert
    EXPECT_FALSE(result.has_value());
}

TEST_F(CalculatorClientTest, MultiplyHttpError)
{
    // Arrange
    mock_client->QueueResponse(HttpResponse{500, R"({"error": "Internal server error"})"});
    CalculatorClient client(std::move(mock_client));

    // Act
    auto result = client.multiply(2, 3);

    // Assert
    EXPECT_FALSE(result.has_value());
}

TEST_F(CalculatorClientTest, MultiplyInvalidJson)
{
    // Arrange
    mock_client->QueueResponse(HttpResponse{200, "not valid json"});
    CalculatorClient client(std::move(mock_client));

    // Act
    auto result = client.multiply(2, 3);

    // Assert
    EXPECT_FALSE(result.has_value());
}

TEST_F(CalculatorClientTest, MultiplyMissingResultField)
{
    // Arrange
    mock_client->QueueResponse(HttpResponse{200, R"({"wrong_field": 42})"});
    CalculatorClient client(std::move(mock_client));

    // Act
    auto result = client.multiply(2, 3);

    // Assert
    EXPECT_FALSE(result.has_value());
}

TEST_F(CalculatorClientTest, MultiplyNegativeNumbers)
{
    // Arrange
    mock_client->QueueResponse(HttpResponse{200, R"({"result": -10})"});
    CalculatorClient client(std::move(mock_client));

    // Act
    auto result = client.multiply(-2, 5);

    // Assert
    ASSERT_TRUE(result.has_value());
    EXPECT_EQ(result.value(), -10);
}
