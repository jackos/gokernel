
### gokernel

## Intro
A very simple kernel that is currently made to run specifically with VS Code notebook API. It takes in a cell that has been executed from the notebook, figures out the order of execution and current order in the notebook, creates a main.go file in a temporary directory and runs it, taking only the outputs from the executing cell and returning the results to VS Code.

The benefits of compilation instead of using an interpreter are: 
- 1 to 1 with compiled go code
- Many programs will run faster (0.2s for small programs)
- Things like re-declaring variables works without issue
- Much greater simplicity, 0 external dependencies
