
# gokernel

## Intro
A very simple kernel that is currently made to run specifically with VS Code notebook API. It takes in a cell that has been executed from the notebook, figures out what cell has been has been executed and changed, creates a main.go file in a temporary directory and runs it, taking only the outputs from the executing cell and returning the results to VS Code.

The benefits of compilation instead of using an interpreter are: 
- 1 to 1 with compiled Go code
- Many programs will run faster (0.2s for small programs)
- Things like rerunnning a cell that declares a variable work without issue
- Much greater simplicity, 0 external dependencies
