// Copyright (c) Zefchain Labs, Inc.
// SPDX-License-Identifier: Apache-2.0

#![cfg_attr(target_arch = "wasm32", no_main)]

mod state;

use std::collections::BTreeSet;

use fungible::Account;
use linera_sdk::{
    base::{AccountOwner, WithContractAbi},
    views::{RootView, View},
    Contract, ContractRuntime, DataBlobHash,
};
use linera_sdk::base::ApplicationId;
use non_fungible::{Message, Nft, NftStatus, NonFungibleTokenAbi, Operation, TokenId};
use universal_solver::UniversalSolverAbi;
use self::state::NonFungibleTokenState;

pub struct NonFungibleTokenContract {
    state: NonFungibleTokenState,
    runtime: ContractRuntime<Self>,
}

linera_sdk::contract!(NonFungibleTokenContract);

impl WithContractAbi for NonFungibleTokenContract {
    type Abi = NonFungibleTokenAbi;
}

impl Contract for NonFungibleTokenContract {
    type Message = Message;
    type InstantiationArgument = ();
    type Parameters = ApplicationId<UniversalSolverAbi>;

    async fn load(runtime: ContractRuntime<Self>) -> Self {
        let state = NonFungibleTokenState::load(runtime.root_view_storage_context())
            .await
            .expect("Failed to load state");
        NonFungibleTokenContract { state, runtime }
    }

    async fn instantiate(&mut self, _state: Self::InstantiationArgument) {
        // Validate that the application parameters were configured correctly.
        self.runtime.application_parameters();
        self.state.num_minted_nfts.set(0);
    }

    async fn execute_operation(&mut self, operation: Self::Operation) -> Self::Response {
        match operation {
            Operation::Mint {
                minter,
                name,
                blob_hash,
                token,
                price,
                id,
                chain_owner,
                chain_minter,
                description
            } => {
                // self.check_account_authentication(minter);
                self.mint(minter, name, blob_hash, token, price, id, chain_owner, chain_minter, description).await;
            }

            Operation::Transfer {
                source_owner,
                token_id,
                target_account,
                chain_owner,
                buy_from_token,
                to_token,
                amount
            } => {
                // self.check_account_authentication(source_owner);

                let mut nft = self.get_nft(&token_id).await;
                // change chain owner
                nft.chain_owner = chain_owner.clone();
                // self.check_account_authentication(nft.owner);
                let call_swap = universal_solver::Operation::Swap {
                    from_token: buy_from_token,
                    to_token,
                    amount,
                    destination_address: chain_owner.clone(),
                };

                let universal_solver_id = self.universal_solver_id();
                self.runtime.call_application(false, universal_solver_id, &call_swap);

                self.transfer(nft, target_account).await;
            }

            Operation::Claim {
                source_account,
                token_id,
                target_account,
            } => {
                // self.check_account_authentication(source_account.owner);

                if source_account.chain_id == self.runtime.chain_id() {
                    let nft = self.get_nft(&token_id).await;
                    // self.check_account_authentication(nft.owner);

                    self.transfer(nft, target_account).await;
                } else {
                    self.remote_claim(source_account, token_id, target_account)
                }
            }

            Operation::ListNftForSale {
                token_id,
                chain_owner
            } => {
                let nft = self.get_nft(&token_id).await;
                self.list_nft_for_sale(nft, chain_owner).await;
            }
        }
    }

    async fn execute_message(&mut self, message: Message) {
        match message {
            Message::Transfer {
                mut nft,
                target_account,
            } => {
                let is_bouncing = self
                    .runtime
                    .message_is_bouncing()
                    .expect("Message delivery status has to be available when executing a message");
                if !is_bouncing {
                    nft.owner = target_account.owner;
                }

                self.add_nft(nft).await;
            }

            Message::Claim {
                source_account,
                token_id,
                target_account,
            } => {
                self.check_account_authentication(source_account.owner);

                let nft = self.get_nft(&token_id).await;
                self.check_account_authentication(nft.owner);

                self.transfer(nft, target_account).await;
            }
        }
    }

    async fn store(mut self) {
        self.state.save().await.expect("Failed to save state");
    }
}

impl NonFungibleTokenContract {

    fn universal_solver_id(&mut self) -> ApplicationId<UniversalSolverAbi> {
        self.runtime.application_parameters()
    }

    /// Verifies that a transfer is authenticated for this local account.
    fn check_account_authentication(&mut self, owner: AccountOwner) {
        match owner {
            AccountOwner::User(address) => {
                assert_eq!(
                    self.runtime.authenticated_signer(),
                    Some(address),
                    "The requested transfer is not correctly authenticated."
                )
            }
            AccountOwner::Application(id) => {
                assert_eq!(
                    self.runtime.authenticated_caller_id(),
                    Some(id),
                    "The requested transfer is not correctly authenticated."
                )
            }
        }
    }

    /// Transfers the specified NFT to another account.
    /// Authentication needs to have happened already.
    async fn transfer(&mut self, mut nft: Nft, target_account: Account) {
        self.remove_nft(&nft).await;
        nft.status = NftStatus::Sold;
        if target_account.chain_id == self.runtime.chain_id() {
            nft.owner = target_account.owner;
            self.add_nft(nft).await;
        } else {
            let message = Message::Transfer {
                nft,
                target_account,
            };

            self.runtime
                .prepare_message(message)
                .with_tracking()
                .send_to(target_account.chain_id);
        }
    }

    async fn get_nft(&self, token_id: &TokenId) -> Nft {
        self.state
            .nfts
            .get(token_id)
            .await
            .expect("Failure in retrieving NFT")
            .expect("NFT {token_id} not found")
    }

    async fn mint(&mut self, owner: AccountOwner, name: String, blob_hash: DataBlobHash,
                  token: String, // ETH, SOL
                  price: String, // 0.05 [token]
                  id: u64, // specific chain nft id
                  chain_minter: String, // chain nft minter
                  chain_owner: String,
                  description: String
    ) {
        self.runtime.assert_data_blob_exists(blob_hash);
        let token_id = Nft::create_token_id(
            &self.runtime.chain_id(),
            &self.runtime.application_id().forget_abi(),
            &name,
            &owner,
            &blob_hash,
            *self.state.num_minted_nfts.get(),
            &token,
            price.clone(),
            id,
            &chain_owner,
            &chain_minter
        )
        .expect("Failed to serialize NFT metadata");

        self.add_nft(Nft {
            token_id,
            owner,
            name,
            minter: owner,
            blob_hash,
            token,
            price,
            id,
            chain_owner,
            chain_minter,
            description,
            status: NftStatus::OnSale,
        })
        .await;

        let num_minted_nfts = self.state.num_minted_nfts.get_mut();
        *num_minted_nfts += 1;
    }

    fn remote_claim(
        &mut self,
        source_account: Account,
        token_id: TokenId,
        target_account: Account,
    ) {
        let message = Message::Claim {
            source_account,
            token_id,
            target_account,
        };
        self.runtime
            .prepare_message(message)
            .with_authentication()
            .send_to(source_account.chain_id);
    }

    async fn add_nft(&mut self, nft: Nft) {
        let token_id = nft.token_id.clone();
        let owner = nft.owner;
        let _id = nft.id;

        self.state
            .nfts
            .insert(&token_id, nft.clone())
            .expect("Error in insert statement");
        if let Some(owned_token_ids) = self
            .state
            .owned_token_ids
            .get_mut(&owner)
            .await
            .expect("Error in get_mut statement")
        {
            owned_token_ids.insert(token_id.clone());
        } else {
            let mut owned_token_ids = BTreeSet::new();
            owned_token_ids.insert(token_id.clone());
            self.state
                .owned_token_ids
                .insert(&owner, owned_token_ids)
                .expect("Error in insert statement");
        }

         self
        .state
        .blob_token_ids
        .insert(&_id, nft.token_id.clone())
        .expect("Error in get_mut statement")

    }

    async fn list_nft_for_sale(&mut self, mut nft: Nft, chain_owner: String){
        nft.status = NftStatus::OnSale;
        nft.chain_owner = chain_owner;
        self.state
            .nfts
            .insert(&nft.token_id, nft.clone())
            .expect("Error in insert statement");
    }

    async fn remove_nft(&mut self, nft: &Nft) {
        self.state
            .nfts
            .remove(&nft.token_id)
            .expect("Failure removing NFT");
        let owned_token_ids = self
            .state
            .owned_token_ids
            .get_mut(&nft.owner)
            .await
            .expect("Error in get_mut statement")
            .expect("NFT set should be there!");

        owned_token_ids.remove(&nft.token_id);

            self
            .state
            .blob_token_ids
            .remove(&nft.id)
            .expect("Error in get_mut statement")

    }
}
