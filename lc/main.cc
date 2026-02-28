#include <iostream>

#include <vector>
#include <string>
#include <algorithm>
#include <ranges>
#include <map>

struct Person
{
    std::string name;
    int age;
    int height;
};

int main()
{
    std::vector<Person> people = {
        {"Alice", 30, 60},
        {"zob", 25, 58},
        {"Hank", 25, 58},
        {"Charlie", 45, 83}};

    
    auto filtered = people | std::views::filter([](const Person &p)
                                                { return p.age > 24; }) |
                    std::ranges::to<std::vector>();

    std::ranges::sort(
        filtered,
        [](const Person &a, const Person &b)
        {
            return std::make_tuple(-a.height, a.name) < std::make_tuple(-b.height, b.name);
        });

    for (const Person &p : filtered)
    {
        std::cout << p.name << ":" << p.height << std::endl;
    }

    std::map<int,std::vector<Person>> m;
    for(const Person& p : people){
        m[p.age].push_back(p);
    }

    for(const auto& [k,v] : m){
        
        std::cout<<k<<":"<<v.size()<<":"
    }



