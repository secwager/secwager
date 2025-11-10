#pragma once

namespace secwager
{
    namespace foo
    {
        class Calculator
        {

        public:
            Calculator() = default;
            ~Calculator() = default;

            int multiply(int a, int b);
        };

    }
}