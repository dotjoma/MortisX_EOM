# Contributing to MortisX EOM

Thanks for your interest in contributing! Here are quick guidelines to keep contributions clean and manageable.

## How to Contribute

1. **Fork the repository** on GitHub.
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/<your-username>/MortisX_EOM.git
   cd MortisX_EOM
   ```
3. **Create a branch** for your fix or feature:
   ```bash
   git checkout -b feature/my-cool-feature
   ```
4. **Make changes & test**:
   - Ensure code compiles cleanly:
     ```bash
     go test ./...
     npm run build:frontend
     ```
   - Do **NOT** commit personal configs or credentials (`bot.json` must remain untracked).
5. **Commit your changes**:
   ```bash
   git commit -m "feat: add support for XYZ"
   ```
6. **Push to your fork** and submit a **Pull Request** to the `main` branch.

## Pull Request Guidelines

- Keep PRs focused on a single feature or bug fix.
- Describe what was changed and why in the PR description.
- Be respectful and constructive in reviews and discussions.
