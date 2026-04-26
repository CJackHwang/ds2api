## Vercel Deployment Setup

To deploy DS2API on Vercel, please follow the steps below. Ensure that your team has authorized the GitHub repository to access Vercel.

1. **Fork the repository:** Start by forking this repository to your GitHub account.

2. **Authorize Vercel access to GitHub:** You must authorize the Vercel app in your GitHub account to enable seamless deployment. If you have not done this, please visit [Authorize Vercel](https://vercel.com/git/authorize?team=cjack's%20projects&slug=cjacks-projects&teamId=team_yoKsHf2u1Bex01ufY7srIByP&type=github&job=%7B%22headInfo%22%3A%7B%22sha%22%3A%22ef27d79290ed53e55c3a2aec9c7cca0bc6923037%22%7D%2C%22id%22%3A%22QmdGT2JtKCFGejmbQixMUYxJtVBwWbVfmrtEt5aRjwwwEM%22%2C%22org%22%3A%22CJackHwang%22%2C%22prId%22%3A317%2C%22repo%22%3A%22ds2api%22%7D) to begin the authorization process.

3. **Import the Project on Vercel:** In Vercel, add a new project by selecting your forked repository.

4. **Set Environment Variables:** You need to configure the following environment variables in Vercel:
   - `DS2API_ADMIN_KEY`: The key for admin access.
   - (Recommended) `DS2API_CONFIG_JSON`: Base64 encoded version of your `config.json`. You can generate it locally using:
     ```bash
     base64 < config.json | tr -d '\n'
     ```

5. **Deploy the Application:** Click on the deploy button to start the deployment process. Ensure that you monitor the build logs for errors and address them as needed.