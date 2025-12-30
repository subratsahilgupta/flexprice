### Current System
Currently we have a temporal workflow which takes list of subscriptions and process them. WHile this proces
It check following things
1. Check if subscription is draft
2. Calculate the billing peroiods
3. For each billing period it 
    - Creates and draft invoice
    - FInalizezs the invoice
4. For each invoice we attempt sync to external vendor
5. For each invoice we attempt payment
6. Check subscripiton for cancellation
7. Update the current billing period of the subs

### Issues
Finalizing, syncing and then attempting the payment should not be concern of the update billing period cron.
Any invoice related issue will hamper the issue of updating the billing period.
This maybe create issue of duplicate invoice when i retry the workflow

### New proposed architecture change
1. Create one separate workflow for invoice related activities
2. Update billing period cron should only update the billing period
3. New invoice workflow will do following this
    - Finalize invoice
    - Sync invoices to external system
    - Attemp payment on the invoice
4. Subscription Update billing period workflow will do following
    - Check for darft
    - Calculate the billing peroiods
    - For each billing period it 
        - Creates and draft invoice
    - Check cancellation
    - Update the current billing period of the subs
    - For each invoice 
        - trigger new workflow of invoice

### Things to consider
1. Failing of invoice creation in subscription workflow should hamper subscription upgrade
    - i.e should be update subscription billing period before creating invoices

2. Every activity should be retryable, i.e I can retry from anywhere
3. Even if one acitivity fails in workflow, fail whole workflow.


### Things to keep in mind
1. Follow best practices
2. Follow existing patterns of repository
3. Use already written functions from service layer, no business logic in activites.
4. Cut different acitvity for finalize invoice step and move it to invoice workflow.

